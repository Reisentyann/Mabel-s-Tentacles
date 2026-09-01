package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/describer-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/search"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
)

// searchFiles 检索文件元数据：?q=&tag=&type=&creator=&scope=&color=&deleted=&page=&size=
func (s *Server) searchFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sq := search.Query{
		Text:           q.Get("q"),
		Tags:           q["tag"],
		FileType:       q.Get("type"),
		Creator:        q.Get("creator"),
		Scope:          q.Get("scope"),
		IncludeDeleted: q.Get("deleted") == "true",
		Page:           1,
		Size:           20,
	}
	if color := q.Get("color"); color != "" {
		sq.Attributes = map[string]any{"color": color}
	}
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p >= 1 {
		sq.Page = p
	}
	if sz, err := strconv.Atoi(q.Get("size")); err == nil && sz >= 1 && sz <= 200 {
		sq.Size = sz
	}

	if s.searcher == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0, "page": sq.Page, "size": sq.Size})
		return
	}

	items, total, err := s.searcher.Search(r.Context(), sq)
	if err != nil {
		slog.Error("search files failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": sq.Page, "size": sq.Size})
}

// getFileMetadata 获取单个文件的元数据：?path=<相对 data 路径>
func (s *Server) getFileMetadata(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	m, err := s.repo.GetMetadata(r.Context(), p)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "metadata not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

type describeRequest struct {
	Path        string          `json:"path"`
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Tags        []string        `json:"tags"`
	FileType    *string         `json:"file_type"`
	Mode        string          `json:"mode"` // replace（默认）| append
	Attributes  json.RawMessage `json:"attributes"`
}

// describeFile 手动补充文件描述/标签/属性（llm 轨：前缀闸门 + 追加模式）。
func (s *Server) describeFile(w http.ResponseWriter, r *http.Request) {
	var req describeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	target, err := service.ResolvePath(s.cfg.DataDir, req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if info, err := os.Stat(target); err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// 既有元数据：追加模式与 llm 字段合并都需要
	var existing map[string]any
	var oldDesc string
	if s.repo != nil {
		if m, err := s.repo.GetMetadata(r.Context(), req.Path); err == nil && m != nil {
			existing = describer.AttrsFromJSON(m.Attributes)
			if m.Description != nil {
				oldDesc = *m.Description
			}
		}
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	if req.Mode == "append" && oldDesc != "" && desc != "" {
		desc = oldDesc + "\n\n" + desc
	}
	var descPtr *string
	if desc != "" {
		descPtr = &desc
	}

	// llm 语义字段：前缀闸门（受控词表 + sp-llm-* 放行，其余丢弃并 WARN）
	if len(req.Attributes) > 0 {
		var in map[string]any
		if err := json.Unmarshal(req.Attributes, &in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid attributes JSON")
			return
		}
		kept, dropped := describer.SanitizeLLM(in)
		for _, d := range dropped {
			slog.Warn("describe attribute dropped by prefix gate", "path", req.Path, "key", d)
		}
		if len(kept) > 0 {
			if _, ok := kept["llm-source"]; !ok {
				kept["llm-source"] = describer.LLMSourceAgent
			}
			existing = describer.MergeLLM(existing, kept, time.Now())
		}
	}

	m := &repo.FileMetadata{
		FilePath:    req.Path,
		Title:       req.Title,
		Description: descPtr,
		Tags:        req.Tags,
		FileType:    req.FileType,
		Attributes:  describer.JSONFromAttrs(existing),
	}
	if s.repo != nil {
		if err := s.repo.UpsertMetadata(r.Context(), m); err != nil {
			slog.Error("describe file failed", "path", req.Path, "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	slog.Info("describe file ok", "path", req.Path, "mode", req.Mode)
	writeJSON(w, http.StatusOK, map[string]string{"message": "metadata updated"})
}

type copyRequest struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// copyFile 复制文件内容 + 元数据。
func (s *Server) copyFile(w http.ResponseWriter, r *http.Request) {
	var req copyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Source == "" || req.Target == "" {
		writeError(w, http.StatusBadRequest, "source and target are required")
		return
	}
	if req.Source == req.Target {
		writeError(w, http.StatusBadRequest, "source and target must differ")
		return
	}

	content, err := service.SafeRead(s.cfg.DataDir, req.Source)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := service.SafeWrite(s.cfg.DataDir, req.Target, string(content)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.repo != nil {
		if err := s.repo.CopyMetadata(r.Context(), req.Source, req.Target, "", ""); err != nil {
			// 源文件可能没有元数据，复制失败不致命；回填目标基础元数据，避免检索遗漏
			slog.Warn("copy metadata failed, recording basic meta", "source", req.Source, "error", err)
			ft, mt := service.InferFileMeta(req.Target)
			ext := service.InferExtension(req.Target)
			cs := service.ChecksumSHA256(content)
			size := int64(len(content))
			_ = s.repo.UpsertMetadata(r.Context(), &repo.FileMetadata{
				FilePath:  req.Target,
				Scope:     "global",
				FileType:  &ft,
				MimeType:  &mt,
				Extension: &ext,
				SizeBytes: &size,
				Checksum:  &cs,
			})
		}
	}
	slog.Info("copy file ok", "source", req.Source, "target", req.Target)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully copied " + req.Source + " to " + req.Target})
}
