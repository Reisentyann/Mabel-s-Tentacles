// 文件：mcp-server-go/internal/tools/chaos/chaos.go —— 混沌机 MCP 工具面：遍历 chaos-go 注册表自动挂载每个娱乐功能
// 修改：2026-09-21（日期由 fresh-header.ps1 刷新）

// Package chaos 是混沌机（chaos-go）的 MCP 工具面。
//
// 装配是泛化的：遍历 chaos-go 的功能注册表，按每个 Feature 的 Params 声明
// 动态生成一个同名 MCP 工具。**新增娱乐功能 = 在 chaos-go 里加一个自注册
// 文件**——本包与 all.go 都不用动，工具自动出现。
package chaos

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
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

		// 特殊钩子：针对产生本地归档文件的混沌机工具（如 jm_comic 下载），
		// 执行零拷贝落盘入库并签发 Mabel 下载短链，深度融入 Mabel 管理生命周期。
		if f.Name() == "jm_comic" {
			postProcessJMComic(ctx, deps, sessionID, out)
		}

		slog.Info("chaos feature ok", "feature", f.Name(),
			"session", sessionID, "duration", time.Since(begin).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, f.Name(), "", "success", "", params)
		return tools.Result(out), nil
	}
}

// postProcessJMComic 对 JMComic 导出的压缩归档执行零拷贝入库与短链生成。
func postProcessJMComic(ctx context.Context, deps tools.Deps, sessionID string, out map[string]any) {
	if out == nil {
		return
	}
	archivePath, _ := out["archive_path"].(string)
	if archivePath == "" {
		return
	}

	info, err := os.Stat(archivePath)
	if err != nil || info.IsDir() {
		return
	}

	if deps.Manager == nil || deps.Store == nil {
		return
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

	// 2. 向 Mabel 数据库预留占位行，获取该逻辑路径的权威 UUID
	uuid, err := deps.Store.ReserveMeta(ctx, logicKey)
	if err != nil {
		slog.Warn("jmcomic reserve meta failed", "path", logicKey, "error", err)
		return
	}

	// 3. 计算 Mabel 的物理存储绝对路径：data/<uuid前2位>/<uuid><ext>
	targetAbs, err := deps.Manager.StorageAbs(uuid, logicKey)
	if err != nil {
		slog.Warn("jmcomic compute storage path failed", "uuid", uuid, "path", logicKey, "error", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		slog.Warn("jmcomic mkdir target dir failed", "target", targetAbs, "error", err)
		return
	}

	// 4. 零拷贝搬移（同一磁盘卷瞬间 rename；跨卷自动退化为流式移动）
	if err := moveOrCopyFile(archivePath, targetAbs); err != nil {
		slog.Warn("jmcomic move file to mabel storage failed", "src", archivePath, "dst", targetAbs, "error", err)
		return
	}

	out["storage_uuid"] = uuid
	out["logic_path"] = logicKey

	// 5. 编排机接管异步分析（T1：落库元数据、计算 Hash、喂索引机）
	if deps.Orch != nil {
		title, _ := out["title"].(string)
		deps.Orch.Submit(core.Event{
			Kind:      core.KindWrite,
			Path:      logicKey,
			SessionID: sessionID,
			Actor:     tools.Actor(ctx),
			Agent: &core.AgentMeta{
				Title:       common.StrPtr(title),
				Description: common.StrPtr("Downloaded via JMComic chaos machine"),
				FileType:    common.StrPtr("application/zip"),
			},
		})
	}

	// 6. 生成 24 小时有效的 Mabel 极简短链（/d/{code}）与防篡改票据下载链接
	var dlURL string
	if deps.Cfg != nil {
		dlBase := strings.TrimRight(deps.Cfg.API.DownloadBaseURL, "/")
		if dlBase == "" {
			dlBase = deps.Cfg.Server.BaseURL
		}
		if shortURL, err := service.IssueShortURL(ctx, deps.Store, dlBase, logicKey, uuid, 24*time.Hour); err == nil {
			dlURL = shortURL
		}
	}
	if dlURL == "" {
		dlURL = deps.Manager.IssueDownloadURL(logicKey, uuid, 0)
	}

	if dlURL != "" {
		out["download_url"] = dlURL
		out["message"] = fmt.Sprintf("已成功下载并收录入库: %s\nMabel 专属下载短链（24小时有效）：\n%s", logicKey, dlURL)
	}
}

// moveOrCopyFile 优先原子 Rename，失败（如跨卷）回退到流式拷贝并清理源文件。
func moveOrCopyFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	_ = in.Close()
	_ = os.Remove(src)
	return nil
}
