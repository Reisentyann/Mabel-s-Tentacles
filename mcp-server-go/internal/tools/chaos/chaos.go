// 文件：mcp-server-go/internal/tools/chaos/chaos.go —— 混沌机 MCP 工具面：遍历 chaos-go 注册表自动挂载每个娱乐功能
// 修改：2026-09-23（日期由 fresh-header.ps1 刷新）

// Package chaos 是混沌机（chaos-go）的 MCP 工具面。
//
// 装配是泛化的：遍历 chaos-go 的功能注册表，按每个 Feature 的 Params 声明
// 动态生成一个同名 MCP 工具。**新增娱乐功能 = 在 chaos-go 里加一个自注册
// 文件**——本包与 all.go 都不用动，工具自动出现。
package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	chaoslib "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/truerandom"
	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/service"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	injectEntropy(chaoslib.Default(), deps.Cfg)
	for _, f := range chaoslib.Features() {
		tool := mcp.NewTool(f.Name(), toolOptions(f)...)
		s.AddTool(tool, handler(deps, f))
	}
}

// injectEntropy 按配置注入真随机熵源（chaos.true_random.source = jinan/anu/off）。
// off 或未配置 = 不注入，此时 true_random 工具如实报"熵源未配置"（不回退）。
func injectEntropy(c *chaoslib.Chaos, cfg *config.Config) {
	if cfg == nil {
		return
	}
	tr := cfg.Chaos.TrueRandom
	timeout := time.Duration(tr.TimeoutSeconds) * time.Second
	switch tr.Source {
	case "lfdr":
		c.SetEntropySource(truerandom.NewLFDR(tr.Endpoint, timeout))
		slog.Info("chaos true_random source enabled", "source", "lfdr", "endpoint", tr.Endpoint)
	case "jinan":
		c.SetEntropySource(truerandom.NewJinan(tr.Endpoint, timeout))
		slog.Info("chaos true_random source enabled", "source", "jinan", "endpoint", tr.Endpoint)
	case "anu":
		c.SetEntropySource(truerandom.NewANU(tr.Endpoint, timeout))
		slog.Info("chaos true_random source enabled", "source", "anu", "endpoint", tr.Endpoint)
	}
}

// toolOptions 把功能的 Param 声明翻译成 MCP 工具入参 schema。
func toolOptions(f chaoslib.Feature) []mcp.ToolOption {
	opts := []mcp.ToolOption{mcp.WithDescription(f.Description())}
	for _, p := range f.Params() {
		prop := []mcp.PropertyOption{mcp.Description(p.Description)}
		if p.Required {
			prop = append(prop, mcp.Required())
		}
		switch p.Type {
		case chaoslib.ParamNumber:
			opts = append(opts, mcp.WithNumber(p.Name, prop...))
		case chaoslib.ParamBool:
			opts = append(opts, mcp.WithBoolean(p.Name, prop...))
		default:
			opts = append(opts, mcp.WithString(p.Name, prop...))
		}
	}
	return opts
}

// handler 生成某个功能的工具处理器：收集声明过的入参 → 交混沌机执行。
func handler(deps tools.Deps, f chaoslib.Feature) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID := tools.SessionID(ctx)
		begin := time.Now()

		args := req.GetArguments()
		params := chaoslib.Params{}
		for _, p := range f.Params() {
			if v, ok := args[p.Name]; ok {
				params[p.Name] = v
			}
		}

		out, err := chaoslib.Default().Run(f.Name(), params)
		if err != nil {
			slog.Error("chaos feature failed", "feature", f.Name(), "session", sessionID,
				"error", err, "duration", time.Since(begin).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, f.Name(), "", "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		// 特殊钩子：针对产生本地归档文件的混沌机工具（jm_comic 下载），
		// 交给管理机导入并签发 Mabel 下载短链，接入文件生命周期。
		operationPath := ""
		if f.Name() == "jm_comic" {
			if err := postProcessJMComic(ctx, deps, sessionID, begin, out); err != nil {
				slog.Error("jm_comic ingest failed", "feature", f.Name(),
					"session", sessionID, "error", err, "duration", time.Since(begin).String())
				tools.RecordOperation(ctx, deps.Store, sessionID, f.Name(), "", "failed", err.Error(), params)
				return tools.ResultError(err.Error()), nil
			}
			operationPath, _ = out["logic_path"].(string)
		}

		slog.Info("chaos feature ok", "feature", f.Name(),
			"session", sessionID, "duration", time.Since(begin).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, f.Name(), operationPath, "success", "", params)
		return tools.Result(out), nil
	}
}

// postProcessJMComic 把 JMComic 导出的压缩归档交给管理机入库，再由编排机
// 异步完成描述与索引。混沌机不再自行 ReserveMeta 或拼接 data 物理路径。
func postProcessJMComic(ctx context.Context, deps tools.Deps, sessionID string, begin time.Time, out map[string]any) error {
	if out == nil {
		return nil
	}
	archivePath, _ := out["archive_path"].(string)
	if archivePath == "" {
		out["ingest_status"] = "skipped_no_archive"
		return nil // raw 格式没有归档文件，保留下载器原始结果。
	}

	if deps.Manager == nil || deps.Store == nil {
		return fmt.Errorf("jm_comic: manager/store not wired; archive=%s", archivePath)
	}

	// 1. 确定入库逻辑路径：优先用入参指定 save_name，否则用 comics/<filename>
	logicKey, _ := out["save_name"].(string)
	logicKey = strings.TrimSpace(logicKey)
	if logicKey == "" {
		fname, _ := out["archive_filename"].(string)
		if fname == "" {
			fname = filepath.Base(archivePath)
		}
		logicKey = "comics/" + fname
	}
	logicKey = strings.ReplaceAll(logicKey, "\\", "/")
	logicKey = strings.TrimPrefix(logicKey, "/")

	// 2. 与其他 MCP 写入口采用同一用户键空间与写权限口径。
	sc, err := tools.ScopeWrite(ctx, logicKey)
	if err != nil {
		return fmt.Errorf("jm_comic target path: %w", err)
	}
	logicKey = sc.Key
	if denied, reason := tools.CanFile(ctx, deps.Store, logicKey, true); denied {
		return fmt.Errorf("jm_comic target %q denied: %s", logicKey, reason)
	}

	// 3. 管理机是唯一入库口：它负责 Reserve、uuid 派生物理路径与文件搬运。
	receipt, err := deps.Manager.ImportFile(ctx, logicKey, archivePath)
	if err != nil {
		return fmt.Errorf("jm_comic import %q: %w", logicKey, err)
	}

	out["storage_uuid"] = receipt.UUID
	out["logic_path"] = logicKey
	out["stored_bytes"] = receipt.SizeBytes
	out["ingest_status"] = "imported"
	// 不把服务端临时路径继续暴露给 MCP 调用方；归档已由管理机接管。
	out["archive_path"] = ""
	out["save_dir"] = ""

	// 4. 编排机接管异步分析（T1：落库元数据、计算 Hash、喂索引机）
	if deps.Orch != nil {
		title, _ := out["title"].(string)
		deps.Orch.Submit(core.Event{
			Kind:      core.KindWrite,
			Path:      logicKey,
			SessionID: sessionID,
			Actor:     tools.Actor(ctx),
			Agent: &core.AgentMeta{
				Title:       common.StrPtr(title),
				Description: common.StrPtr(jmDescription(out)),
				Tags:        jmTags(out),
				FileType:    common.StrPtr("application/zip"),
				Attributes:  jmAttributes(out),
			},
		})
	}

	// 5. 生成 24 小时有效的 Mabel 极简短链（/d/{code}）与防篡改票据下载链接
	var dlURL string
	if deps.Store != nil && deps.Cfg != nil {
		dlBase := strings.TrimRight(deps.Cfg.API.DownloadBaseURL, "/")
		if dlBase == "" {
			dlBase = deps.Cfg.Server.BaseURL
		}
		if shortURL, err := service.IssueShortURL(ctx, deps.Store, dlBase, logicKey, receipt.UUID, 24*time.Hour); err == nil {
			dlURL = shortURL
		}
	}
	if dlURL == "" {
		dlURL = deps.Manager.IssueDownloadURL(logicKey, receipt.UUID, 0)
	}

	if dlURL != "" {
		out["download_url"] = dlURL
		out["message"] = fmt.Sprintf("已成功下载并收录入库: %s\nMabel 专属下载短链（24小时有效）：\n%s", logicKey, dlURL)
	}
	slog.Info("jm_comic archive imported", "feature", "jm_comic", "path", logicKey,
		"uuid", receipt.UUID, "bytes", receipt.SizeBytes, "session", sessionID,
		"duration", time.Since(begin).String())
	return nil
}

// jmDescription 给文件详情补一段可读的来源摘要；详细字段仍放在
// sp-cod-jm-* attributes 中，供索引机按 id/作者/标签/页数检索。
func jmDescription(out map[string]any) string {
	title, _ := out["title"].(string)
	id := fmt.Sprint(out["id"])
	if id == "<nil>" || id == "" {
		return "Downloaded via JMComic chaos machine"
	}
	if title == "" {
		return fmt.Sprintf("JMComic %s", id)
	}
	return fmt.Sprintf("JMComic %s · %s", id, title)
}

func jmTags(out map[string]any) []string {
	return stringSlice(out["tags"])
}

// jmAttributes 只取 JMComic 结构化详情，不把 archive_path/save_dir 等内部
// 工作路径带进元数据。字段值保持 string/number/array，indexer 可直接建桶。
func jmAttributes(out map[string]any) map[string]any {
	attrs := map[string]any{}
	if id, ok := out["id"]; ok && id != nil {
		attrs["sp-cod-jm-id"] = fmt.Sprint(id)
	}
	for _, field := range []string{"type", "title", "page_count", "episode_count", "pub_date", "update_date"} {
		if value, ok := out[field]; ok && value != nil {
			attrs["sp-cod-jm-"+field] = value
		}
	}
	for _, field := range []string{"author", "tags", "actors", "works"} {
		if values := stringSlice(out[field]); values != nil {
			attrs["sp-cod-jm-"+field] = values
		}
	}
	if episodes := jmEpisodeIDs(out["episodes"]); episodes != nil {
		attrs["sp-cod-jm-episode-ids"] = episodes
	}
	return attrs
}

func stringSlice(value any) []string {
	switch values := value.(type) {
	case string:
		if strings.TrimSpace(values) == "" {
			return nil
		}
		return []string{values}
	case []string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text := strings.TrimSpace(value); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func jmEpisodeIDs(value any) []string {
	episodes, ok := value.([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		item, ok := episode.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := item["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
