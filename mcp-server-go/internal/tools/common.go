// 文件：mcp-server-go/internal/tools/common.go —— 工具共享层：Result / SessionID / RecordOperation / Principal 授权助手 / DownloadURL
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

func Result(v map[string]any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(b))
}

func ResultError(message string) *mcp.CallToolResult {
	return Result(map[string]any{"success": false, "message": message})
}

func SessionID(ctx context.Context) string {
	if s := server.ClientSessionFromContext(ctx); s != nil {
		return s.SessionID()
	}
	return ""
}

func RecordOperation(ctx context.Context, st repo.Store, sessionID, tool, filePath, status, errMsg string, params map[string]any) {
	if st == nil {
		return
	}
	if err := st.RecordOperation(ctx, sessionID, tool, filePath, status, errMsg, params); err != nil {
		slog.Warn("record operation failed", "tool", tool, "error", err)
	}
}

// Principal 工具侧取请求主体（mcp.AuthMiddleware 注入；nil = 通道异常，
// 敏感操作一律拒绝——fail-closed）。
func Principal(ctx context.Context) *authz.Principal {
	return authz.PrincipalFrom(ctx)
}

// Actor Principal → 编排机 Actor 投影（owner 落库标识）。
func Actor(ctx context.Context) core.Actor {
	a := core.Actor{}
	if p := authz.PrincipalFrom(ctx); p != nil {
		a.Name = p.Name
	}
	return a
}

// Deny 统一拒绝回执：WARN 留痕（谁在哪个工具试图动哪个文件、为什么拒绝
// ——安全审计第一现场）+ 给 agent 的人话原因（可自纠错）。
func Deny(ctx context.Context, tool, path, reason string) *mcp.CallToolResult {
	p := authz.PrincipalFrom(ctx)
	slog.Warn("mcp permission denied",
		"tool", tool, "path", path, "principal", p.Subject(),
		"session", SessionID(ctx), "reason", reason)
	return ResultError("权限不足: " + reason)
}

// CanFile 工具侧对目标文件做 authz 判定。denied=true 时调用方直接
// 返回 Deny 的结果。
func CanFile(ctx context.Context, st repo.Store, path string, write bool) (denied bool, reason string) {
	p := authz.PrincipalFrom(ctx)
	if p == nil {
		return true, "未认证主体"
	}
	if st == nil {
		return false, ""
	}
	m, err := st.GetMetadata(ctx, path)
	if err != nil {
		if write {
			if ok, r := authz.CanWrite(p, authz.FileACL{}); !ok {
				return true, r
			}
			return false, ""
		}
		if ok, r := authz.CanRead(p, authz.FileACL{}); !ok {
			return true, r
		}
		return false, ""
	}
	if m.IsDeleted {
		return true, "文件已被软删除"
	}
	if write {
		if ok, r := authz.CanWrite(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID)); !ok {
			return true, r
		}
		return false, ""
	}
	if ok, r := authz.CanRead(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID)); !ok {
		return true, r
	}
	return false, ""
}

// StrPtr 空串→nil 的指针语义转换已收敛 common.StrPtr（core 侧 strPtr、
// repo 侧 derefStr 同步退役），本包不再持有副本。

// DownloadURL 构造文件的对外下载地址。download_base_url 未配置时返回空串（不返回下载链接）。
func DownloadURL(cfg *config.Config, filePath string) string {
	base := strings.TrimRight(cfg.API.DownloadBaseURL, "/")
	if base == "" {
		return ""
	}
	u := base + "/api/files/download?path=" + url.QueryEscape(filePath)
	if cfg.API.AccessToken != "" {
		u += "&token=" + url.QueryEscape(cfg.API.AccessToken)
	}
	return u
}

// ===== owner 键空间（多用户隔离批次 2026-09-08）=====

// OwnerMark 保留段前缀：DB 键 = ~<主体名>/逻辑路径（S3 bucket / Unix home
// 形态）。~ 不在 usernamePattern 字符集内，保留段不可伪装。agent 寻址
// 语言：无前缀 = 自己空间（工具层自动拼接，agent 无感知）；显式 ~A/ =
// 跨用户寻址（只读语义——可见性由行内 ACL 授权判定，写操作一律拒绝）。
const OwnerMark = "~"

// ScopedPath 寻址解析结果：最终 DB 键 + 目标空间主体 + 是否自己空间。
type ScopedPath struct {
	Key   string // 最终 DB 键（~owner/逻辑路径）
	Owner string // 目标空间主体名
	Self  bool   // 是否自己空间
}

// ScopePath 解析 agent 给的寻址路径 → 键空间规则：
//   - "笔记.txt" / "~/笔记.txt"：自己空间 → ~<主体>/笔记.txt
//   - "~A/笔记.txt"：A 的空间（跨用户寻址；写侧调用方须拒）
//
// 未认证主体 / 空路径 / 裸 "~" / 坏段 → err（fail-closed）。
func ScopePath(ctx context.Context, path string) (ScopedPath, error) {
	p := authz.PrincipalFrom(ctx)
	if p == nil {
		return ScopedPath{}, errors.New("未认证主体")
	}
	if path == "" {
		return ScopedPath{}, errors.New("路径为空")
	}
	first, rest, hasRest := strings.Cut(path, "/")
	switch {
	case first == OwnerMark: // "~/..." → 自己
		if !hasRest || rest == "" {
			return ScopedPath{}, errors.New(`"~"后缺少路径段`)
		}
		return selfScope(p.Name, rest), nil
	case strings.HasPrefix(first, OwnerMark): // "~A/..."
		target := strings.TrimPrefix(first, OwnerMark)
		if target == "" || !hasRest || rest == "" {
			return ScopedPath{}, errors.New("跨用户寻址格式：~<用户名>/<路径>")
		}
		return ScopedPath{Key: OwnerMark + target + "/" + rest, Owner: target, Self: target == p.Name}, nil
	default: // 无前缀 → 自己
		return selfScope(p.Name, path), nil
	}
}

func selfScope(actor, rest string) ScopedPath {
	return ScopedPath{Key: OwnerMark + actor + "/" + rest, Owner: actor, Self: true}
}

// ScopeWrite 写侧寻址：跨用户一律拒绝（MCP 工具不代人写；admin 管理走
// HTTP admin 端点）。副本/移动的目标键同口径（拿走即拥有 → 只入自己空间）。
func ScopeWrite(ctx context.Context, path string) (ScopedPath, error) {
	sc, err := ScopePath(ctx, path)
	if err != nil {
		return sc, err
	}
	if !sc.Self {
		return sc, errors.New("跨用户写入禁止（~用户名/ 前缀仅支持只读寻址）")
	}
	return sc, nil
}

// OwnerPrefix 主体的键空间前缀（~<主体名>/）——列目录侧过滤用。
func OwnerPrefix(ctx context.Context) (string, error) {
	p := authz.PrincipalFrom(ctx)
	if p == nil {
		return "", errors.New("未认证主体")
	}
	return OwnerMark + p.Name + "/", nil
}
