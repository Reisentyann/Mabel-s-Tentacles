// 文件：mcp-server-go/internal/api/metadata.go —— 元数据端点：搜索 / 查看元数据 / 描述（编排机同步入口）/ 复制（KindCopy 事件）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
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

	if p := principalOf(r); p != nil && !p.IsAdmin() {
		sq.ViewerName = p.Name
		sq.ViewerGroups = p.GroupIDs
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
	// 读授权：不可读的文件按不存在隐藏（404），存在性不泄露
	if !s.canActFile(w, r, p, "read", false) {
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
	Mode        string          `json:"mode"`       // replace（默认）| append
	Visibility  string          `json:"visibility"` // public / group / private（空 = 保留既有）
	GroupID     *int64          `json:"group_id"`   // group 可见性的组
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
	if s.orch == nil {
		writeError(w, http.StatusInternalServerError, "orchestrator unavailable")
		return
	}
	// 写授权：describe 是改元数据的动手操作，owner/admin 之外拒绝
	if !s.canActFile(w, r, req.Path, "describe", true) {
		return
	}

	// 编排机同步入口（与 MCP describe_file 同一份实现）：LLMStore 闸门 +
	// 单次 Upsert + 喂索引——llm-* 是可索引字段，原先不喂的漂移洞已堵
	attrs := map[string]any{}
	if len(req.Attributes) > 0 {
		if err := json.Unmarshal(req.Attributes, &attrs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid attributes JSON")
			return
		}
	}
	actor := core.Actor{}
	if p := principalOf(r); p != nil {
		actor.Name = p.Name
	}
	res, derr := s.orch.Describe(r.Context(), core.DescribeRequest{
		Path:        req.Path,
		Title:       req.Title,
		Description: req.Description,
		Tags:        req.Tags,
		FileType:    req.FileType,
		Mode:        req.Mode,
		Visibility:  req.Visibility,
		GroupID:     req.GroupID,
		Actor:       actor,
		Attributes:  attrs,
	}, "")
	if derr != nil {
		slog.Error("describe file failed", "path", req.Path, "error", derr)
		writeError(w, http.StatusInternalServerError, derr.Error())
		return
	}
	slog.Info("describe file ok", "path", req.Path, "mode", req.Mode, "rejected", len(res.Rejected))
	resp := map[string]string{"message": "metadata updated"}
	if len(res.Rejected) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"message": "metadata updated", "rejected": res.Rejected})
		return
	}
	writeJSON(w, http.StatusOK, resp)
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

	// 物理复制走管理机（读源 + 入库目标：逻辑键 → uuid 派生随机物理路径）
	if s.manager == nil {
		writeError(w, http.StatusInternalServerError, "manager not wired")
		return
	}
	of, err := s.manager.OpenByLogic(r.Context(), req.Source)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	content, rerr := io.ReadAll(of.Content)
	of.Content.Close()
	if rerr != nil {
		writeError(w, http.StatusInternalServerError, rerr.Error())
		return
	}
	if _, err := s.manager.Write(r.Context(), req.Target, string(content)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 源文件读授权：看不到的文件不允许复制（404 隐藏存在性）
	if !s.canActFile(w, r, req.Source, "copy-read", false) {
		return
	}

	owner := ""
	if p := principalOf(r); p != nil {
		owner = p.Name
	}
	if s.repo != nil {
		// 副本归操作者（拿走即拥有，copied_from 保留谱系）
		if err := s.repo.CopyMetadata(r.Context(), req.Source, req.Target, owner, ""); err != nil {
			// 源文件可能没有元数据，复制失败不致命：编排机 KindCopy 事件的
			// 执行器会从盘上重建目标元数据并喂索引（COALESCE 保留 copied_from 等复制列）
			slog.Warn("copy metadata failed, orchestrator will rebuild target meta", "source", req.Source, "error", err)
		}
	}
	if s.orch != nil {
		s.orch.Submit(core.Event{Kind: core.KindCopy, Path: req.Target, Actor: core.Actor{Name: owner}})
	}
	slog.Info("copy file ok", "source", req.Source, "target", req.Target)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Successfully copied " + req.Source + " to " + req.Target})
}
