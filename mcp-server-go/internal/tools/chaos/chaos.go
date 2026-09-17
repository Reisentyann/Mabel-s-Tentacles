// 文件：mcp-server-go/internal/tools/chaos/chaos.go —— 混沌机 MCP 工具面：遍历 chaos-go 注册表自动挂载每个娱乐功能
// 修改：2026-09-17（日期由 fresh-header.ps1 刷新）

// Package chaos 是混沌机（chaos-go）的 MCP 工具面。
//
// 装配是泛化的：遍历 chaos-go 的功能注册表，按每个 Feature 的 Params 声明
// 动态生成一个同名 MCP 工具。**新增娱乐功能 = 在 chaos-go 里加一个自注册
// 文件**——本包与 all.go 都不用动，工具自动出现。
package chaos

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	chaoslib "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/all"
	"github.com/Reisentyann/Mabel-s-Tentacles/chaos-go/truerandom"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
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

		slog.Info("chaos feature ok", "feature", f.Name(),
			"session", sessionID, "duration", time.Since(begin).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, f.Name(), "", "success", "", params)
		return tools.Result(out), nil
	}
}
