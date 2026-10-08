// 文件：mcp-server-go/internal/api/directories.go —— UUID 文件夹 HTTP 接口：列出、受控本地导入和完整 ZIP 下载
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"fmt"
	manager "github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/authz"
	"io"
	"net/http"
	"os"
	"strings"
)

func (s *Server) directoryAccess(w http.ResponseWriter, r *http.Request, id string) bool {
	if s.manager == nil {
		writeError(w, 503, "manager unavailable")
		return false
	}
	d, err := s.manager.LocateDirectory(r.Context(), id)
	if err != nil {
		writeError(w, 404, "directory not found")
		return false
	}
	p := principalOf(r)
	if p == nil || (!p.IsAdmin() && d.Path != "~"+p.Name && !strings.HasPrefix(d.Path, "~"+p.Name+"/")) {
		writeError(w, 404, "directory not found")
		return false
	}
	return true
}
func (s *Server) directoryList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("uuid")
	if !s.directoryAccess(w, r, id) {
		return
	}
	items, err := s.manager.ListDirectory(r.Context(), id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	visible := []manager.DirectoryEntry{}
	for _, e := range items {
		if s.directoryEntryAllowed(r, e) == nil {
			visible = append(visible, e)
		}
	}
	writeJSON(w, 200, map[string]any{"uuid": id, "entries": visible})
}
func (s *Server) directoryEntryAllowed(r *http.Request, e manager.DirectoryEntry) error {
	p := principalOf(r)
	if p == nil {
		return fmt.Errorf("authorization unavailable")
	}
	if p.IsAdmin() {
		return nil
	}
	if e.Kind == "dir" {
		d, err := s.manager.LocateDirectory(r.Context(), e.UUID)
		if err != nil {
			return err
		}
		if d.Path == "~"+p.Name || strings.HasPrefix(d.Path, "~"+p.Name+"/") {
			return nil
		}
		return fmt.Errorf("directory denied")
	}
	metas, err := s.repo.GetMetadataByUUIDs(r.Context(), []string{e.UUID})
	if err != nil {
		return err
	}
	m := metas[e.UUID]
	if m == nil {
		return fmt.Errorf("file unavailable")
	}
	if ok, _ := authz.CanRead(p, authz.ACLOf(m.OwnerID, m.Visibility, m.GroupID)); !ok {
		return fmt.Errorf("file denied")
	}
	return nil
}
func (s *Server) directoryExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("uuid")
	if !s.directoryAccess(w, r, id) {
		return
	}
	f, err := os.CreateTemp("", "mabel-directory-*.zip")
	if err != nil {
		writeError(w, 500, "archive staging failed")
		return
	}
	defer f.Close()
	if err := s.manager.ExportDirectory(r.Context(), id, f, func(e manager.DirectoryEntry) error { return s.directoryEntryAllowed(r, e) }); err != nil {
		writeError(w, 500, "archive failed: "+err.Error())
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		writeError(w, 500, "archive read failed")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="directory.zip"`)
	_, _ = io.Copy(w, f)
}

// 本地导入限 admin，来源不对普通用户暴露任意服务器路径。上传接收端后续接受受控 staging。
func (s *Server) directoryImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("uuid")
	if !s.directoryAccess(w, r, id) {
		return
	}
	var req struct {
		Source string `json:"source"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, 400, "invalid body")
		return
	}
	items, err := s.manager.ImportDirectory(r.Context(), id, req.Source)
	failed := 0
	for _, item := range items {
		if item.Error != "" {
			failed++
			continue
		}
		if ref, e := s.manager.Locate(r.Context(), item.UUID); e == nil && s.orch != nil {
			s.orch.Submit(core.Event{Kind: core.KindWrite, Path: ref.Path, Actor: core.Actor{Name: principalOf(r).Name}})
		}
	}
	if err != nil {
		writeJSON(w, 500, map[string]any{"success": false, "entries": items, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"success": failed == 0, "failed": failed, "entries": items})
}
