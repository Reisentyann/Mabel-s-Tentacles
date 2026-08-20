package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	tree, err := service.ListTree(s.cfg.DataDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tree": tree})
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	if !s.checkAccessToken(r) {
		writeError(w, http.StatusForbidden, "invalid access token")
		return
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

	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(target)+`"`)
	http.ServeFile(w, r, target)
}

func (s *Server) downloadZip(w http.ResponseWriter, r *http.Request) {
	if !s.checkAccessToken(r) {
		writeError(w, http.StatusForbidden, "invalid access token")
		return
	}

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

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="files.zip"`)
	if _, err := service.ZipFiles(s.cfg.DataDir, body.Paths, w); err != nil {
		slog.Error("download zip failed", "error", err)
	}
}
