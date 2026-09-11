// 文件：mcp-server-go/internal/tools/chaos/chaos.go —— 混沌机 MCP 工具面：遍历 chaos-go 注册表自动挂载每个娱乐功能
// 修改：2026-09-11（日期由 fresh-header.ps1 刷新）

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
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	for _, f := range chaoslib.Features() {
		tool := mcp.NewTool(f.Name(), toolOptions(f)...)
		s.AddTool(tool, handler(deps, f))
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
