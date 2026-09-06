// 文件：mcp-server-go/internal/mcp/auth.go —— MCP 通道鉴权：master key（管家，无限权限）/ 外部 key（agent_keys 表，绑定用户）/ 空 key 开发放行
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
)

// AuthMiddleware MCP 通道级信任（docs/权限设计.md）：
//
//  1. master key（.env MCP_API_KEY）：服务器自己的管家，无限权限
//     （Principal{agent-master, Role=admin}）
//  2. 外部 key（agent_keys 表，admin 签发）：绑定一个用户身份，
//     权限=该用户（Principal{agent-external}）——raw key 只在签发时
//     出现一次，库中存 sha256
//  3. master key 未配置：开发放行（启动时已大红 WARN），注入管家主体
//     ——与 require_auth=false 的 HTTP 开发口径同款
//
// 验证失败的每次尝试都留痕（撞 key 是安全第一现场）。
// Principal 经 context 贯通到工具层（mcp-go 的 message ctx 基于
// r.Context() 构建，中间件注入值可达 handler——sse.go WithContext 语义）。
func AuthMiddleware(next http.Handler, masterKey string, st repo.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))

		// 1) 开发放行：master key 未配置（启动 WARN 已提醒）
		if masterKey == "" {
			p := &authz.Principal{Kind: authz.KindAgentMaster, Name: "agent", Role: "admin"}
			next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), p)))
			return
		}

		// 2) 管家：master key 常量时间比对
		if bearer != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(masterKey)) == 1 {
			p := &authz.Principal{Kind: authz.KindAgentMaster, Name: "agent", Role: "admin"}
			next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), p)))
			return
		}

		// 3) 外部 agent：查 agent_keys（hash 直查）→ 绑定用户的实时身份
		if bearer != "" && st != nil {
			sum := sha256.Sum256([]byte(bearer))
			k, err := st.GetAgentKeyByHash(r.Context(), hex.EncodeToString(sum[:]))
			if err == nil && k != nil && k.IsActive {
				if u, uerr := st.GetUserByID(r.Context(), k.PrincipalUID); uerr == nil && u != nil && u.IsActive {
					gids, _ := st.UserGroupIDs(r.Context(), u.ID)
					p := &authz.Principal{
						Kind: authz.KindAgentExternal, UID: u.ID, Name: u.Username,
						Role: u.Role, GroupIDs: gids,
					}
					next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(r.Context(), p)))
					return
				}
			}
		}

		// 4) 拒绝：无效/吊销 key，或绑定的用户已被停用——每次尝试留痕
		slog.Warn("mcp auth failed",
			"ip", common.ClientIP(r), "path", r.URL.Path,
			"error", "invalid or revoked key")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// clientIP 已收敛 common.ClientIP（与 api 中间件同口径的副本退役）。
