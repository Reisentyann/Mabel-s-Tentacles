// 文件：manager-go/intake.go —— 入库域：文件新增的唯一口（逻辑键 + uuid 派生物理随机路径，agent 不接触物理布局）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// intake 域职责（2026-09-08 架构决策落地：物理路径防猜 + 文件 IO 主权收归管理机）：
//
//   - agent 只给逻辑路径（"小说/第一章.txt"——组织语言，DB 唯一键），
//     物理存储路径由本域从 uuid 纯派生（<uuid前2位>/<uuid><ext>），
//     盘面只有随机名——存储位置不可猜测，逻辑视图只在 DB
//   - uuid 生成仍归 DB（gen_random_uuid）：Reserve 落占位行幂等拿 uuid，
//     盘写发生在 uuid 之后（物理路径需要它派生）
//   - 物理路径不存库（纯派生零冗余）；Move 因此变纯 DB 键改——
//     uuid 不变则物理文件根本不用搬
//   - 旧明文存量已清场（旧内容/data-明文存量-20260908），无迁移兼容层
//
// 时序（write_file 全链）：工具层 CanFile 授权 → manager.Write（Reserve →
// 派生 → 落盘）→ 编排机事件 → executor 盘读（storageAbs）→ 描述落库喂索引。
package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
)

// maxIntakeBytes 单次写入内容上限（= describer.MaxFullBytes 单一来源，
// 与分析预算同口径：值得入库的分析粒度）。
const maxIntakeBytes = describer.MaxFullBytes

// ErrInvalidPath 逻辑路径非法（空 / 前导分隔符 / 盘符 / ..段 / 空段）。
var ErrInvalidPath = errors.New("manager: invalid logic path")

// WriteReceipt 入库回执：uuid（后续操作凭证）+ 逻辑键 + 物理相对路径
// （审计/排障用，agent 场景不依赖它）。
type WriteReceipt struct {
	UUID       string `json:"uuid"`
	LogicPath  string `json:"logic_path"`
	StorageRel string `json:"storage_rel"`
	SizeBytes  int64  `json:"size_bytes"`
}

// StoragePathOf 物理存储相对路径（uuid 派生：<uuid前2位>/<uuid><ext>）——
// 位置域核心知识：盘面布局对 agent 不可预测（防猜）；ext 保留自逻辑路径
// （MIME 推导 / describer 引擎要用）。跨平台注意：返回正斜杠形，
// 盘操作前由调用方 filepath.FromSlash / resolve 归一。
func StoragePathOf(uuid, logicPath string) (string, error) {
	if len(uuid) < 2 {
		return "", fmt.Errorf("storage path: bad uuid %q", uuid)
	}
	return path.Join(uuid[:2], uuid+path.Ext(logicPath)), nil
}

// storageAbs 物理相对路径 → dataDir 内绝对路径（防穿越纵深：路径虽是
// 系统派生理论安全，纵深校验不亏——uuid/ext 全来自受控源也挡实现回归）。
func (m *Manager) storageAbs(uuid, logicPath string) (string, error) {
	rel, err := StoragePathOf(uuid, logicPath)
	if err != nil {
		return "", err
	}
	return m.resolve(rel)
}

// validLogicPath 逻辑键规范：非空、无前导分隔符、无盘符、无 . / .. / 空段。
// 逻辑键是 DB 唯一键兼 agent 寻址语言——规范性即寻址可靠性。
func validLogicPath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return ErrInvalidPath
	}
	if strings.Contains(p, ":") {
		return fmt.Errorf("security error: path cannot contain drive letters")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, `\`) {
			return ErrInvalidPath
		}
	}
	return nil
}

// Write 文件入库唯一口：Reserve 占位行（幂等拿 uuid）→ uuid 派生物理路径 →
// 落盘。描述/落库/喂索引归编排机事件流（调用方提交），本口只管"文件在哪
// +内容落盘"。同逻辑键重写 = 覆写（占位行复用既有 uuid，物理路径不变）。
func (m *Manager) Write(ctx context.Context, logicPath, content string) (*WriteReceipt, error) {
	if err := validLogicPath(logicPath); err != nil {
		return nil, err
	}
	if len(content) > maxIntakeBytes {
		return nil, fmt.Errorf("security error: file content exceeds 5MB limit")
	}
	uuid, err := m.store.ReserveMeta(ctx, logicPath)
	if err != nil {
		return nil, fmt.Errorf("reserve meta: %w", err)
	}
	rel, err := StoragePathOf(uuid, logicPath)
	if err != nil {
		return nil, err
	}
	abs, err := m.resolve(rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return &WriteReceipt{UUID: uuid, LogicPath: logicPath, StorageRel: rel, SizeBytes: int64(len(content))}, nil
}

// Modify 修改既有文件（append / overwrite）。行必须已在（无行 = 文件不存在，
// 修改入口不是创建入口——与旧 SafeModify 语义一致，文案沿用）。
func (m *Manager) Modify(ctx context.Context, logicPath, content, mode string) error {
	if err := validLogicPath(logicPath); err != nil {
		return err
	}
	if len(content) > maxIntakeBytes {
		return fmt.Errorf("security error: file content exceeds 5MB limit")
	}
	if mode != "append" && mode != "overwrite" {
		return fmt.Errorf("error: invalid mode '%s', must be 'append' or 'overwrite'", mode)
	}
	row, err := m.store.GetMeta(ctx, logicPath)
	if err != nil {
		return fmt.Errorf("get meta: %w", err)
	}
	if row == nil {
		return fmt.Errorf("error: file '%s' does not exist", logicPath)
	}
	abs, err := m.storageAbs(row.UUID, logicPath)
	if err != nil {
		return err
	}
	if mode == "overwrite" {
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return fmt.Errorf("overwrite file: %w", err)
		}
		return nil
	}
	f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("append file: %w", err)
	}
	return nil
}

// LogicNode 逻辑树节点（与前端树视图结构同构：name/path/type/size/updated）。
type LogicNode struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"` // 逻辑路径（agent 寻址键）
	Type      string       `json:"type"` // dir | file
	SizeBytes int64        `json:"size_bytes,omitempty"`
	UpdatedAt float64      `json:"updated_at,omitempty"`
	Children  []*LogicNode `json:"children,omitempty"`
}

// LogicTree 逻辑视图树：DB 全量行（不含软删）按逻辑路径段组建树。
// 物理随机化后盘上无树可看——"文件在哪"的树状答案只有这里能给。
func (m *Manager) LogicTree(ctx context.Context) ([]*LogicNode, error) {
	root := &LogicNode{}
	since := ""
	for {
		rows, err := m.store.ListMetaPage(ctx, since, 200)
		if err != nil {
			return nil, fmt.Errorf("logic tree scan: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			since = r.Path
			insertLogicNode(root, r.Path, r.SizeBytes, r.UpdatedAt)
		}
	}
	return sortLogicChildren(root.Children), nil
}

// LogicPaths 逻辑路径全集（树展平叶子，字典序）——list_data_files 工具的
// 树源替代（原物理盘枚举退役：盘上只有随机名，无逻辑可枚举）。
func (m *Manager) LogicPaths(ctx context.Context) ([]string, error) {
	since := ""
	out := []string{}
	for {
		rows, err := m.store.ListMetaPage(ctx, since, 200)
		if err != nil {
			return nil, fmt.Errorf("logic paths scan: %w", err)
		}
		if len(rows) == 0 {
			return out, nil
		}
		for _, r := range rows {
			since = r.Path
			out = append(out, r.Path)
		}
	}
}

// insertLogicNode 把一行逻辑路径挂进树（段拆分建目录节点，叶子带计量）。
func insertLogicNode(root *LogicNode, p string, size int64, updated time.Time) {
	segs := strings.Split(p, "/")
	cur := root
	for i, seg := range segs {
		if cur.Children == nil {
			cur.Children = []*LogicNode{}
		}
		var next *LogicNode
		for _, c := range cur.Children {
			if c.Name == seg {
				next = c
				break
			}
		}
		if next == nil {
			next = &LogicNode{Name: seg, Path: strings.Join(segs[:i+1], "/")}
			if i < len(segs)-1 {
				next.Type = "dir" // 中间段 = 目录；末段在下方落 file
			}
			cur.Children = append(cur.Children, next)
		}
		cur = next
	}
	if len(segs) > 0 {
		cur.Type = "file"
		cur.SizeBytes = size
		cur.UpdatedAt = float64(updated.UnixNano()) / 1e9
	}
}

// sortLogicChildren 目录在前、名字稳定序（树视图确定性）。
func sortLogicChildren(nodes []*LogicNode) []*LogicNode {
	sort.Slice(nodes, func(i, j int) bool {
		ti, tj := nodes[i].Type == "dir", nodes[j].Type == "dir"
		if ti != tj {
			return ti
		}
		return nodes[i].Name < nodes[j].Name
	})
	for _, n := range nodes {
		if n.Children != nil {
			n.Children = sortLogicChildren(n.Children)
		}
	}
	return nodes
}
