// 文件：manager-go/fetch.go —— 取件域：uuid 兑换处（Locate/LocateMany 出位置，Open/Read 出内容；软删/幽灵/批量语义钉死）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

// fetch 域职责：uuid 的兑换。调用方（编排机 / MCP 工具 / HTTP）持
// indexer.Query 产出的 uuid 集合来问管理机——进 uuid，出位置或文件本体。
// 索引机是 sink 零出调用，uuid 的搬运永远由调用方完成（架构设计.md 第 2 节
// "uuid 是组件间货币，管理机凭 uuid 取件"）。
//
// 语义钉死（2026-09-05 接口轮）：
//   - Locate：知情性质——DB 无行 → ErrNotFound；软删行照报（IsDeleted=true，
//     元数据可追溯，删不删由调用方决策）；不做盘上存在性检查（盘偏差归
//     Open 的幽灵哨兵与 audit 域巡检）
//   - Open/Read：动手性质——软删 → ErrDeleted（回收站文件不供取内容）；
//     行在盘上无 → ErrGhost（等 T2 对账 3 轮软删收编）；流不设预算——
//     管理机不替调用方做产品决策（agent 读由 MCP 层限流，HTTP 下载全量流）
//   - 批量 LocateMany：缺失的 uuid 不入 map（不报错），调用方对照入参
//     集合找缺；搜索结果一次取齐（免 N+1）
//   - Read = Open + 限读（limit<=0 全量）——两形态并存：流给下载、字节给 agent
//   - 内容读取经 buffer.go 取件缓冲区（命中直出 / 盘读入缓 / stat 新鲜度
//     校验兜绕口 / 大文件旁路直流），实现批次落地
//
// 与 placement 域 Resolve(ctx,uuid)(string,error) 的关系：那是纯路径的
// 便捷钉面；实现批次可改写为 Locate 的薄壳（Locate(uuid).Path），不冲突。
package manager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// FileRef uuid 的取件回执（业务视角：位置 + 展示性元数据，不含内容）。
type FileRef struct {
	UUID      string `json:"uuid"`
	Path      string `json:"path"`  // 相对路径（agent 的组织语言 / 元数据唯一键）
	Scope     string `json:"scope"` // global | user | game
	SizeBytes int64  `json:"size_bytes"`
	MimeType  string `json:"mime_type"`
	IsDeleted bool   `json:"is_deleted"` // 软删行照报（回收站可追溯，取内容才被拒）
}

// OpenedFile Open 的产物：位置回执 + 内容流（调用方负责 Close）。
type OpenedFile struct {
	FileRef
	Content io.ReadCloser
}

// ReadFile Read 的产物：位置回执 + 内容字节。
type ReadFile struct {
	FileRef
	Content []byte
}

// 哨兵错误：调用方凭 errors.Is 区分"查无 / 软删 / 幽灵"与底层 IO 错误，
// 不做字符串匹配。
var (
	// ErrNotFound DB 无此 uuid 的行（组件货币铸造失败，uuid 从何而来需排查）。
	ErrNotFound = errors.New("manager: uuid not found")
	// ErrDeleted 行已软删（回收站文件不供取内容；Locate 仍照报位置）。
	ErrDeleted = errors.New("manager: file soft-deleted")
	// ErrGhost 行在、盘上文件已消失（幽灵元数据，T2 对账 3 轮软删收编中）。
	ErrGhost = errors.New("manager: file gone on disk")
)

// errNoFetch 已退役（取件实现批次 2026-09-06 落地：Locate/LocateMany/
// Open/Read 全链路 + buffer LRU）。

// Locate 凭 uuid 取位置（单个）。软删行照报（IsDeleted=true，回收站可
// 追溯）；DB 无行 → ErrNotFound；不做盘上存在性检查（盘偏差归 Open 的
// 幽灵哨兵与 audit 域巡检）。
func (m *Manager) Locate(ctx context.Context, uuid string) (*FileRef, error) {
	ref, err := m.store.GetMetaByUUID(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("locate %s: %w", uuid, err)
	}
	if ref == nil {
		return nil, ErrNotFound
	}
	return ref, nil
}

// LocateMany 凭 uuid 集合批量取位置。缺失的 uuid 不入返回 map（不报错，
// 调用方对照入参找缺）；软删行照报（带 IsDeleted）。空入参 → 空 map。
func (m *Manager) LocateMany(ctx context.Context, uuids []string) (map[string]*FileRef, error) {
	if len(uuids) == 0 {
		return map[string]*FileRef{}, nil // 空入空出：零依赖语义，接口轮即钉死
	}
	refs, err := m.store.GetMetaByUUIDs(ctx, uuids)
	if err != nil {
		return nil, fmt.Errorf("locate many: %w", err)
	}
	if refs == nil {
		refs = map[string]*FileRef{}
	}
	return refs, nil
}

// Open 凭 uuid 取整个文件（流式）。Content 由调用方负责 Close。
// 哨兵语义：查无 → ErrNotFound；软删 → ErrDeleted；盘上消失 → ErrGhost。
// buffer 命中直出（快照，新鲜度由 buffer 内 stat 对拍兜底）；未命中盘读
// （≤ maxEntry 入缓，超限旁路直流）。
func (m *Manager) Open(ctx context.Context, uuid string) (*OpenedFile, error) {
	ref, err := m.Locate(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if ref.IsDeleted {
		return nil, ErrDeleted // 回收站文件不供取内容（Locate 仍照报位置）
	}

	// 快路径：buffer 命中直出（stat 对拍已在 buffer.get 内完成）
	if content, ok := m.buf.get(uuid); ok {
		return &OpenedFile{FileRef: *ref, Content: io.NopCloser(bytes.NewReader(content))}, nil
	}

	abs, err := m.resolve(ref.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrGhost // 行在、盘上无（T2 对账 3 轮软删收编中）
		}
		return nil, fmt.Errorf("stat: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("'%s' is a directory", ref.Path)
	}

	// 大文件旁路：超单条目上限直流不入缓（防挤占整个容量）
	if info.Size() > m.buf.maxEntry {
		f, err := os.Open(abs)
		if err != nil {
			return nil, err
		}
		return &OpenedFile{FileRef: *ref, Content: f}, nil
	}

	// ≤ maxEntry：盘读 → 入缓（put 复制为快照）→ 从快照出流
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	m.buf.put(uuid, bufEntry{path: abs, size: info.Size(), modTime: info.ModTime(), content: content})
	return &OpenedFile{FileRef: *ref, Content: io.NopCloser(bytes.NewReader(content))}, nil
}

// Read 凭 uuid 取内容字节（Open 的便捷包装：限读聚合）。limit <= 0 =
// 全量；>0 = 截到 limit 字节。哨兵语义与 Open 一致。
func (m *Manager) Read(ctx context.Context, uuid string, limit int64) (*ReadFile, error) {
	of, err := m.Open(ctx, uuid)
	if err != nil {
		return nil, err
	}
	defer of.Content.Close()

	var r io.Reader = of.Content
	if limit > 0 {
		r = io.LimitReader(of.Content, limit)
	}
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return &ReadFile{FileRef: of.FileRef, Content: content}, nil
}
