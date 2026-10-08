// 文件：manager-go/intake.go —— 入库域：文本写入与外部文件导入（逻辑键 + uuid 派生物理随机路径）
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

// intake 域职责（2026-09-08 架构决策落地：物理路径防猜 + 文件 IO 主权收归管理机）：
//
//   - agent 只给逻辑路径（"小说/第一章.txt"——组织语言，DB 唯一键），
//     物理存储路径由本域从 uuid 纯派生（<uuid前2位>/<uuid><ext>），
//     盘面只有随机名——存储位置不可猜测，逻辑视图只在 DB
//   - uuid 生成仍归 DB（gen_random_uuid）：Reserve 原子创建占位行拿 uuid，
//     盘写发生在 uuid 之后（物理路径需要它派生）
//   - 物理路径不存库（纯派生零冗余）；Move 因此变纯 DB 键改——
//     uuid 不变则物理文件根本不用搬
//   - 旧明文存量已清场（旧内容/data-明文存量-20260908），无迁移兼容层
//
// 时序（写入全链）：工具层 CanFile 授权 → manager.Write/ImportFile
// （Reserve → 派生 → 落盘）→ 编排机事件 → executor 盘读（storageAbs）→
// 描述落库喂索引。
package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
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
var ErrDirectoryExists = errors.New("manager: directory already exists")

// WriteReceipt 入库回执：uuid（后续操作凭证）+ 逻辑键 + 物理相对路径
// （审计/排障用，agent 场景不依赖它）。
type WriteReceipt struct {
	UUID       string `json:"uuid"`
	LogicPath  string `json:"logic_path"`
	StorageRel string `json:"storage_rel"`
	SizeBytes  int64  `json:"size_bytes"`
}

// ImportReceipt 外部文件导入回执：与 WriteReceipt 同形，但内容来自已有的
// 本地文件。下载器等内部组件只需把源文件路径交给管理机，不需要知道
// uuid 派生的物理布局。
type ImportReceipt struct {
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

// StorageAbs 物理相对路径 → dataDir 内绝对路径（防穿越纵深：路径虽是
// 系统派生理论安全，纵深校验不亏——uuid/ext 全来自受控源也挡实现回归）。
func (m *Manager) StorageAbs(uuid, logicPath string) (string, error) {
	rel, err := StoragePathOf(uuid, logicPath)
	if err != nil {
		return "", err
	}
	return m.resolve(rel)
}

func (m *Manager) storageAbs(uuid, logicPath string) (string, error) {
	return m.StorageAbs(uuid, logicPath)
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

// Write 新文件入库口：Reserve 原子占位 → uuid 派生物理路径 →
// 落盘。描述/落库/喂索引归编排机事件流（调用方提交），本口只管"文件在哪
// +内容落盘"。同逻辑键（包括软删、幽灵、占位行）拒绝；更新使用 Modify。
func (m *Manager) Write(ctx context.Context, logicPath, content string) (receipt *WriteReceipt, resultErr error) {
	if err := validLogicPath(logicPath); err != nil {
		return nil, err
	}
	if len(content) > maxIntakeBytes {
		return nil, fmt.Errorf("security error: file content exceeds 5MB limit")
	}
	if err := m.ensureFilePathAvailable(ctx, logicPath); err != nil {
		return nil, err
	}
	uuid, err := m.store.ReserveMeta(ctx, logicPath)
	if err != nil {
		return nil, fmt.Errorf("reserve meta: %w", err)
	}
	stored := false
	defer func() {
		if !stored {
			m.recoverFailedIntake(ctx, logicPath, uuid, &resultErr)
		}
	}()
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
	if err := publishNewFile(abs, strings.NewReader(content)); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	stored = true
	if err := m.store.CompleteIntake(ctx, logicPath, uuid); err != nil {
		return nil, fmt.Errorf("confirm stored file: %w", err)
	}
	return &WriteReceipt{UUID: uuid, LogicPath: logicPath, StorageRel: rel, SizeBytes: int64(len(content))}, nil
}

// ImportFile 将一个已有的本地文件纳入管理机：Reserve 占位行 → uuid 派生
// 物理路径 → 迁移或复制文件。调用方只负责提供逻辑路径与源文件路径，
// 不得自行 ReserveMeta、拼接 data 目录或直接写物理盘。
//
// 逻辑键必须未占用。同卷且目标不存在时优先 Rename，避免大文件重复读写；
// 跨卷时退化为排他创建并流式复制。跨卷回退不会删除源文件，便于下载任务保留断点与审计
// 证据；成功 Rename 的源路径则由操作系统完成移动。
func (m *Manager) ImportFile(ctx context.Context, logicPath, sourcePath string) (receipt *ImportReceipt, resultErr error) {
	if err := validLogicPath(logicPath); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return nil, errors.New("import file: source path is empty")
	}

	info, err := os.Lstat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("stat source file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("import file: symbolic links are not accepted")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("import file: source is not a regular file")
	}

	uuid, err := m.store.ReserveMeta(ctx, logicPath)
	if err != nil {
		return nil, fmt.Errorf("reserve meta: %w", err)
	}
	stored := false
	defer func() {
		if !stored {
			m.recoverFailedIntake(ctx, logicPath, uuid, &resultErr)
		}
	}()
	rel, err := StoragePathOf(uuid, logicPath)
	if err != nil {
		return nil, err
	}
	targetAbs, err := m.resolve(rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return nil, fmt.Errorf("create import directory: %w", err)
	}
	if err := transferImportedFile(sourcePath, targetAbs); err != nil {
		return nil, fmt.Errorf("store imported file: %w", err)
	}
	stored = true
	if err := m.store.CompleteIntake(ctx, logicPath, uuid); err != nil {
		return nil, fmt.Errorf("confirm stored file: %w", err)
	}
	return &ImportReceipt{
		UUID:       uuid,
		LogicPath:  logicPath,
		StorageRel: rel,
		SizeBytes:  info.Size(),
	}, nil
}

// RecoverPendingIntakes 收敛服务中断期间留下的入库状态：盘上有完整普通文件
// 则确认发布，盘上不存在或不是普通文件则归档占位并释放原名称。
func (m *Manager) RecoverPendingIntakes(ctx context.Context) (completed, archived int, err error) {
	items, err := m.store.ListPendingIntakes(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list pending intakes: %w", err)
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return completed, archived, err
		}
		abs, err := m.storageAbs(item.UUID, item.LogicPath)
		if err != nil {
			return completed, archived, err
		}
		info, statErr := os.Lstat(abs)
		if statErr == nil && info.Mode().IsRegular() {
			if err := m.store.CompleteIntake(ctx, item.LogicPath, item.UUID); err != nil {
				return completed, archived, err
			}
			completed++
			continue
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return completed, archived, statErr
		}
		if err := m.store.ArchiveFailedIntake(ctx, item.LogicPath, item.UUID); err != nil {
			return completed, archived, err
		}
		archived++
	}
	return completed, archived, nil
}

// recoverFailedIntake 失败记录归档而非删除，UUID 与扩展名不变，部分内容仍可对账。
func (m *Manager) recoverFailedIntake(ctx context.Context, logicPath, uuid string, resultErr *error) {
	if *resultErr == nil {
		return
	}
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := m.store.ArchiveFailedIntake(recoveryCtx, logicPath, uuid); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("archive failed intake (path remains reserved): %w", err))
	}
}

// transferImportedFile 先尝试同卷移动；目标存在拒绝，跨卷时排他创建。
// 不在回退路径删除源文件：临时下载产物可继续作为断点与审计证据。
func transferImportedFile(sourcePath, targetPath string) error {
	in, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer in.Close()

	return publishNewFile(targetPath, in)
}

// publishNewFile 正式路径只在完整写入并同步后出现。硬链接发布排他且不覆盖，
// 临时文件保留作证据；不支持硬链接的文件系统明确失败，不降级到非原子覆盖。
func publishNewFile(targetPath string, src io.Reader) error {
	out, err := os.OpenFile(targetPath+".partial", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, src)
	var syncErr error
	if copyErr == nil {
		syncErr = out.Sync()
	}
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Link(targetPath+".partial", targetPath); err != nil {
		return fmt.Errorf("publish file: %w", err)
	}
	// 临时目录项仅作为写入证据，不参与读取和启动发布判断。
	return nil
}

// Modify 修改既有文件（append / overwrite）。行必须已在（无行 = 文件不存在，
// 修改入口不是创建入口——与旧 SafeModify 语义一致，文案沿用）。
func (m *Manager) Modify(ctx context.Context, logicPath, content, mode string) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	ref, err := m.locateByLogic(ctx, logicPath)
	if err != nil {
		return err
	}
	return m.modifyRef(ctx, ref, content, mode)
}

func (m *Manager) modifyRef(ctx context.Context, ref *FileRef, content, mode string) error {
	logicPath := ref.Path
	if err := validLogicPath(logicPath); err != nil {
		return err
	}
	if len(content) > maxIntakeBytes {
		return fmt.Errorf("security error: file content exceeds 5MB limit")
	}
	if mode != "append" && mode != "overwrite" {
		return fmt.Errorf("error: invalid mode '%s', must be 'append' or 'overwrite'", mode)
	}
	if ref.IsDeleted {
		return ErrDeleted
	}
	if err := m.requirePublished(ctx, ref.UUID); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := m.storageAbs(ref.UUID, logicPath)
	if err != nil {
		return err
	}
	in, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrGhost
		}
		return fmt.Errorf("open original: %w", err)
	}
	info, err := in.Stat()
	if err != nil {
		in.Close()
		return err
	}
	if !info.Mode().IsRegular() {
		in.Close()
		return fmt.Errorf("modify requires regular file")
	}
	out, err := os.CreateTemp(filepath.Dir(abs), ".modify-*")
	if err != nil {
		in.Close()
		return err
	}
	// 未发布暂存文件保留供对账，正式文件不截断。
	if mode == "append" {
		_, err = io.Copy(out, in)
	}
	closeErr := in.Close()
	if err == nil {
		_, err = io.WriteString(out, content)
	}
	if err == nil {
		err = out.Chmod(info.Mode().Perm())
	}
	if err == nil {
		err = out.Sync()
	}
	err = errors.Join(err, closeErr, out.Close())
	if err != nil {
		return fmt.Errorf("prepare modification: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replaceFile(out.Name(), abs); err != nil {
		return fmt.Errorf("publish modification: %w", err)
	}
	m.buf.drop(ref.UUID)
	return nil
}

// CreateDirectory 创建逻辑空目录。目录只存于元数据；物理 dataDir 始终只保存 UUID 文件。
func (m *Manager) CreateDirectory(ctx context.Context, logicPath string) error {
	if err := validLogicPath(logicPath); err != nil {
		return err
	}
	for _, prefix := range logicPrefixes(logicPath) {
		if row, err := m.store.GetMeta(ctx, prefix); err != nil {
			return fmt.Errorf("get meta: %w", err)
		} else if row != nil {
			return ErrKeyExists
		}
	}
	if exists, err := m.store.DirectoryExists(ctx, logicPath); err != nil {
		return fmt.Errorf("get directory: %w", err)
	} else if exists {
		return ErrDirectoryExists
	}
	for _, prefix := range logicPrefixes(logicPath) {
		err := m.store.CreateDirectory(ctx, prefix)
		if err != nil && !errors.Is(err, ErrDirectoryExists) {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureFilePathAvailable(ctx context.Context, logicPath string) error {
	for _, prefix := range logicPrefixes(logicPath) {
		if prefix == logicPath {
			exists, err := m.store.DirectoryExists(ctx, prefix)
			if err != nil {
				return fmt.Errorf("get directory: %w", err)
			}
			if exists {
				return ErrKeyExists
			}
			continue
		}
		if row, err := m.store.GetMeta(ctx, prefix); err != nil {
			return fmt.Errorf("get meta: %w", err)
		} else if row != nil {
			return fmt.Errorf("manager: path parent is a file: %s", prefix)
		}
	}
	return nil
}

func logicPrefixes(p string) []string {
	parts := strings.Split(p, "/")
	prefixes := make([]string, len(parts))
	for i := range parts {
		prefixes[i] = strings.Join(parts[:i+1], "/")
	}
	return prefixes
}

// ErrKeyExists 目标逻辑键已被占用（Write/ImportFile/Move 拒绝覆盖——键空间唯一性
// 由 DB UNIQUE 兜底，本哨兵是人话口径）。
var ErrKeyExists = errors.New("manager: logic key already exists; choose another path, or use modify_file/patch_file to update an existing file")

// MoveReceipt 键改回执：uuid + 原键 + 新键 + 物理位是否随 ext 变化搬移。
type MoveReceipt struct {
	UUID        string `json:"uuid"`
	From        string `json:"from"`
	To          string `json:"to"`
	StorageMove bool   `json:"storage_move"`
}

// Move 逻辑键改（文件管理域 2026-09-08；intake 域设计红利兑现）：
//
//   - uuid 不变、行事实（描述/归属/谱系）不变、索引键（uuid）不动——
//     统一描述管线零触发（KindMove 事件在执行器早退）
//   - ext 不变 = 纯 DB 键改，零盘操作；ext 变化 = 物理位随派生规则
//     rename（同卷原子，uuid 分层目录不变）
//   - 谱系 moved_from 记原键（行内列，元数据端点自然带出——谱系查询
//     零新端点）
//   - 无行 / 软删行拒（ErrNotFound / ErrDeleted 哨兵）；目标占用拒
//     （ErrKeyExists）
func (m *Manager) Move(ctx context.Context, from, to string) (*MoveReceipt, error) {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	if err := validLogicPath(from); err != nil {
		return nil, err
	}
	if err := validLogicPath(to); err != nil {
		return nil, err
	}
	if from == to {
		return nil, fmt.Errorf("error: source and target are the same")
	}
	row, err := m.store.GetMeta(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("get meta: %w", err)
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if row.IsDeleted {
		return nil, ErrDeleted
	}

	// 键改（谱系 + uuid 回执归 Store 一手落——repo 侧 UNIQUE 兜底并发竞态）
	if err := m.requirePublished(ctx, row.UUID); err != nil {
		return nil, err
	}
	if err := m.ensureFilePathAvailable(ctx, to); err != nil {
		return nil, err
	}
	if path.Ext(from) != path.Ext(to) {
		if _, ok := m.store.(MoveRecoveryStore); !ok {
			return nil, fmt.Errorf("manager: recoverable extension move unavailable")
		}
		abs, err := m.storageAbs(row.UUID, from)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("move source is not regular")
		}
	}
	uuid, err := m.store.MoveMeta(ctx, from, to)
	if err != nil {
		return nil, err
	}

	// 物理位随 ext 变化才动（同 uuid 派生：分层目录恒同，rename 同卷原子）
	oldRel, err := StoragePathOf(uuid, from)
	if err != nil {
		return nil, err
	}
	newRel, err := StoragePathOf(uuid, to)
	if err != nil {
		return nil, err
	}
	moved := oldRel != newRel
	if moved {
		if err := m.finishStorageMove(MoveOperation{UUID: uuid, From: from, To: to}); err != nil {
			return nil, fmt.Errorf("move pending recovery: %w", err)
		}
		if err := m.store.(MoveRecoveryStore).CompleteMove(ctx, uuid, to); err != nil {
			return nil, fmt.Errorf("confirm move: %w", err)
		}
	}
	m.buf.drop(uuid)
	return &MoveReceipt{UUID: uuid, From: from, To: to, StorageMove: moved}, nil
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
	dirs, err := m.store.ListDirectoryPaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("logic directory scan: %w", err)
	}
	for _, dir := range dirs {
		insertLogicDir(root, dir)
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

func insertLogicDir(root *LogicNode, p string) {
	cur := root
	for i, seg := range strings.Split(p, "/") {
		var next *LogicNode
		for _, child := range cur.Children {
			if child.Name == seg {
				next = child
				break
			}
		}
		if next == nil {
			next = &LogicNode{Name: seg, Path: strings.Join(strings.Split(p, "/")[:i+1], "/"), Type: "dir"}
			cur.Children = append(cur.Children, next)
		}
		if next.Type != "file" {
			next.Type = "dir"
		}
		cur = next
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
