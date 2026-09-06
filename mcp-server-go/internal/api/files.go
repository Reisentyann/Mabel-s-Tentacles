// 文件：mcp-server-go/internal/api/files.go —— 文件端点：可见性裁剪的目录树 / 单文件下载（自证：静态 token 或 JWT）/ zip 打包
// 修改：2026-09-06（日期由 fresh-header.ps1 刷新）

package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// listFiles 目录树（权限批次：非 admin 按可见性裁剪——看不到的文件
// 连文件名都不出现，空目录随之剪枝）。
func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	tree, err := service.ListTree(s.cfg.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tree == nil {
		tree = []*service.FileNode{}
	}
	if p := principalOf(r); p != nil && !p.IsAdmin() && s.repo != nil {
		tree = s.filterTree(r, tree, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tree": tree})
}

// filterTree 按 CanRead 裁剪树：无元数据的文件对非 admin 隐藏
// （归属不明不让看），目录无幸存子节点则整枝剪掉。
func (s *Server) filterTree(r *http.Request, nodes []*service.FileNode, p *authz.Principal) []*service.FileNode {
	paths := collectPaths(nodes, nil)
	metas, err := s.repo.GetMetadataByPaths(r.Context(), paths)
	if err != nil {
		slog.Warn("list files fetch metadata failed, tree unfiltered", "error", err)
		return nodes
	}
	out := make([]*service.FileNode, 0, len(nodes))
	for _, n := range nodes {
		if keepTree(r, s, n, p, metas) {
			out = append(out, n)
		}
	}
	return out
}

// keepTree 节点保留判定（目录递归改写 Children，文件按可见性）。
// 无元数据文件按存量口径对待（authz：空 ACL = public 可读）——与
// read_file/download 的 CanFile 判定同口径，不另立规矩。
func keepTree(r *http.Request, s *Server, n *service.FileNode, p *authz.Principal, metas map[string]*repo.FileMetadata) bool {
	if n.Type == "dir" {
		kids := make([]*service.FileNode, 0, len(n.Children))
		for _, c := range n.Children {
			if keepTree(r, s, c, p, metas) {
				kids = append(kids, c)
			}
		}
		n.Children = kids
		return len(kids) > 0
	}
	m := metas[n.Path]
	if m == nil {
		return true // 无行 = 存量口径 public（与 CanRead 空 ACL 一致）
	}
	ok, _ := authz.CanRead(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID))
	return ok
}

// collectPaths 收集树里全部文件路径（批量联表用）。
func collectPaths(nodes []*service.FileNode, acc []string) []string {
	for _, n := range nodes {
		if n.Type == "dir" {
			acc = collectPaths(n.Children, acc)
		} else {
			acc = append(acc, n.Path)
		}
	}
	return acc
}

// downloadFile 单文件下载（公共路由自证）：优先静态 access_token
// （配置后供 agent 直发链接的过渡口径），否则要求 JWT 并做 CanRead。
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	if !s.checkAccessToken(r) {
		if s.cfg.API.RequireAuth {
			p := s.selfAuth(w, r)
			if p == nil {
				return // 响应已写（401/403/404）
			}
			path := r.URL.Query().Get("path")
			if !s.canActFile(w, r, path, "download", false) {
				return
			}
		}
		// require_auth=false（本地开发显式关闭）：匿名放行，与全局面口径一致
	}

	p := r.URL.Query().Get("path")
	target, err := service.ResolvePath(s.cfg.DataDir, p)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// 软删除拦截 + 下载计数（best-effort，元数据缺失则放行）
	if s.repo != nil {
		if m, err := s.repo.GetMetadata(r.Context(), p); err == nil && m.IsDeleted {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		_ = s.repo.IncrementDownloadCount(r.Context(), p)
	}

	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(target)+`"`)
	http.ServeFile(w, r, target)
}

// selfAuth 公共端点自证（下载）：解析 Bearer JWT → 实时主体。
// 失败时已写响应，返回 nil。
func (s *Server) selfAuth(w http.ResponseWriter, r *http.Request) *authz.Principal {
	claims, err := service.ParseToken(s.cfg.Security.SecretKey, bearerToken(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return nil
	}
	p := s.loadPrincipal(r.Context(), claims)
	if p == nil {
		slog.Warn("download auth rejected: account disabled or missing",
			"user", claims.Subject, "ip", common.ClientIP(r), "path", r.URL.Path)
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return nil
	}
	return p
}

// downloadZip zip 打包下载（JWT 保护）：逐路径 CanRead，拒绝的跳过并留痕。
func (s *Server) downloadZip(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "no paths provided")
		return
	}

	// 可见性过滤：非 admin 只打包自己读得到的路径（无行按存量 public 口径）
	if p := principalOf(r); p != nil && !p.IsAdmin() && s.repo != nil {
		metas, err := s.repo.GetMetadataByPaths(r.Context(), body.Paths)
		if err != nil {
			slog.Warn("download zip fetch metadata failed", "error", err)
		}
		allowed := make([]string, 0, len(body.Paths))
		for _, path := range body.Paths {
			m := metas[path]
			if m == nil {
				allowed = append(allowed, path)
				continue
			}
			if ok, _ := authz.CanRead(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID)); ok {
				allowed = append(allowed, path)
			} else {
				s.denyLog(r, "download-zip", path, "他人不可读文件")
			}
		}
		body.Paths = allowed
		if len(body.Paths) == 0 {
			writeError(w, http.StatusForbidden, "no downloadable files in request")
			return
		}
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="files.zip"`)
	if _, err := service.ZipFiles(s.cfg.DataDir, body.Paths, w); err != nil {
		slog.Error("download zip failed", "error", err)
	}
}
