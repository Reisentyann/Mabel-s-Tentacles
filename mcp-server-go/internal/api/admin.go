// 文件：mcp-server-go/internal/api/admin.go —— 管理端点（admin 专属）：用户封停 / 组管理 / 外部 agent key 签发与吊销
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
)

// —— 用户管理 ——

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.repo.ListUsers(r.Context())
	if err != nil {
		slog.Error("admin list users failed", "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (s *Server) setUserActive(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("username")
	var body struct {
		Active bool `json:"active"`
	}
	if err := decodeJSON(r, &body); err != nil || name == "" {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.repo.SetUserActive(r.Context(), name, body.Active); err != nil {
		slog.Error("admin set user active failed", "username", name, "active", body.Active, "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	// 安全事件必须留痕：谁在何时启停了谁（封号是权限体系里最重的操作）
	slog.Info("admin set user active",
		"operator", principalOf(r).Subject(), "username", name, "active", body.Active)
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// —— 组管理 ——

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	g, err := s.repo.CreateGroup(r.Context(), body.Name)
	if err != nil {
		slog.Error("admin create group failed", "name", body.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "create group failed")
		return
	}
	slog.Info("admin group created", "operator", principalOf(r).Subject(), "group", g.Name, "id", g.ID)
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.repo.ListGroups(r.Context())
	if err != nil {
		slog.Error("admin list groups failed", "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": groups})
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	if err := s.repo.DeleteGroup(r.Context(), id); err != nil {
		slog.Error("admin delete group failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	slog.Info("admin group deleted", "operator", principalOf(r).Subject(), "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

func (s *Server) addGroupMember(w http.ResponseWriter, r *http.Request) {
	if !s.groupMemberAction(w, r, true) {
		return
	}
}

func (s *Server) removeGroupMember(w http.ResponseWriter, r *http.Request) {
	s.groupMemberAction(w, r, false)
}

// groupMemberAction 入组/退组的共用实现；返回 false = 已写响应。
func (s *Server) groupMemberAction(w http.ResponseWriter, r *http.Request, add bool) bool {
	gid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return false
	}
	var body struct {
		Username string `json:"username"`
	}
	if add { // DELETE 也读 body？退组用路径参数更顺：/members/{username}
		if err := decodeJSON(r, &body); err != nil || body.Username == "" {
			writeError(w, http.StatusBadRequest, "username is required")
			return false
		}
	} else {
		body.Username = r.PathValue("username")
		if body.Username == "" {
			writeError(w, http.StatusBadRequest, "username is required")
			return false
		}
	}
	u, err := s.repo.GetUserByUsername(r.Context(), body.Username)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return false
	}
	if add {
		err = s.repo.AddGroupMember(r.Context(), gid, u.ID)
	} else {
		err = s.repo.RemoveGroupMember(r.Context(), gid, u.ID)
	}
	if err != nil {
		slog.Error("admin group member action failed", "group", gid, "user", u.Username, "add", add, "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return false
	}
	slog.Info("admin group member changed",
		"operator", principalOf(r).Subject(), "group", gid, "user", u.Username, "added", add)
	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
	return true
}

// —— 外部 agent key 管理 ——

// generateAgentKey 生成 raw key（32 字节随机，hex 呈现）——只在签发响应里
// 出现这一次，库与列表永不回传。
func generateAgentKey() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = "mak-" + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return
}

func (s *Server) issueAgentKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Username string `json:"username"` // 绑定的用户身份（权限=该用户）
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" || body.Username == "" {
		writeError(w, http.StatusBadRequest, "name and username are required")
		return
	}
	u, err := s.repo.GetUserByUsername(r.Context(), body.Username)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	raw, keyHash, err := generateAgentKey()
	if err != nil {
		slog.Error("generate agent key failed", "error", err)
		writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}
	k, err := s.repo.CreateAgentKey(r.Context(), body.Name, keyHash, u.ID)
	if err != nil {
		slog.Error("admin create agent key failed", "name", body.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "create key failed")
		return
	}
	// 审计留痕但不落 raw key——凭证本体只进签发响应
	slog.Info("admin agent key issued",
		"operator", principalOf(r).Subject(), "key_name", k.Name, "principal", u.Username, "id", k.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": k.ID, "name": k.Name, "username": u.Username,
		"created_at": k.CreatedAt,
		"key":        raw, // 仅此一次
		"hint":       "配置到 MCP 客户端的 Authorization: Bearer <key>；关闭本页后无法再次查看",
	})
}

func (s *Server) listAgentKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.repo.ListAgentKeys(r.Context())
	if err != nil {
		slog.Error("admin list agent keys failed", "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": keys})
}

func (s *Server) revokeAgentKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key id")
		return
	}
	if err := s.repo.RevokeAgentKey(r.Context(), id); err != nil {
		slog.Error("admin revoke agent key failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	slog.Warn("admin agent key revoked", "operator", principalOf(r).Subject(), "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"message": "revoked"})
}

// —— 授权判定助手（api 各端点共用；拒绝一律留痕） ——

// canActFile 对单个文件做 CanRead/CanWrite。ok=false 时已写好
// 响应（403/404）并落拒绝日志，调用方直接 return。
func (s *Server) canActFile(w http.ResponseWriter, r *http.Request, path, op string, write bool) bool {
	p := principalOf(r)
	if s.repo == nil || p == nil {
		writeError(w, http.StatusInternalServerError, "authorization unavailable")
		return false
	}
	m, err := s.repo.GetMetadata(r.Context(), path)
	if err != nil {
		// 无行：读侧按 404 隐藏；写侧按"无主遗产"口径拒绝（authz 内原因）
		if write {
			ok, reason := authz.CanWrite(p, authz.FileACL{})
			if !ok {
				s.denyLog(r, op, path, reason)
				writeError(w, http.StatusForbidden, "permission denied: "+reason)
				return false
			}
			return true
		}
		writeError(w, http.StatusNotFound, "metadata not found")
		return false
	}
	var ok bool
	var reason string
	if write {
		ok, reason = authz.CanWrite(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID))
	} else {
		ok, reason = authz.CanRead(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID))
	}
	if !ok {
		s.denyLog(r, op, path, reason)
		// 读侧 404 隐藏存在性；写侧 403 明确拒绝
		if write {
			writeError(w, http.StatusForbidden, "permission denied: "+reason)
		} else {
			writeError(w, http.StatusNotFound, "metadata not found")
		}
		return false
	}
	return true
}

// denyLog 拒绝留痕（埋日志：谁试图动谁的东西，安全审计第一现场）。
func (s *Server) denyLog(r *http.Request, op, path, reason string) {
	p := principalOf(r)
	slog.Warn("permission denied",
		"op", op, "path", path, "user", p.Subject(),
		"ip", common.ClientIP(r), "reason", reason)
}
