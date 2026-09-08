// 文件：mcp-server-go/internal/tools/movefile/movefile.go —— MCP 工具 move_file：逻辑键改（管理机 Move；文件管理域 2026-09-08）
// 修改：2026-09-08（日期由 fresh-header.ps1 刷新）

package movefile

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
)

func init() {
	tools.Register(register)
}

func register(s *server.MCPServer, deps tools.Deps) {
	tool := mcp.NewTool("move_file",
		mcp.WithDescription("Move (rename) a file to a new logical path. The file keeps its identity: uuid, description, tags and lineage are unchanged; only the key changes. Renaming with a different extension also moves the stored file; same extension is a pure database rename. Source can address another user's file as ~username/path if you can read it, but the target must be in your own space."),
		mcp.WithString("source",
			mcp.Required(),
			mcp.Description("Source logical path (your space by default, or ~username/path to address another user's readable file)."),
		),
		mcp.WithString("target",
			mcp.Required(),
			mcp.Description("Target logical path (must be in your own space)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source, err := req.RequireString("source")
		if err != nil {
			return tools.ResultError("invalid source: " + err.Error()), nil
		}
		target, err := req.RequireString("target")
		if err != nil {
			return tools.ResultError("invalid target: " + err.Error()), nil
		}
		if source == target {
			return tools.ResultError("source and target must differ"), nil
		}

		sessionID := tools.SessionID(ctx)
		start := time.Now()
		params := map[string]any{"source": source, "target": target}

		// 键空间（owner 隔离批次）：源可跨用户只读寻址（移动是对源动手的
		// 写操作，跨用户由行内 owner 矩阵拒）；目标仅自己空间
		scS, serrS := tools.ScopePath(ctx, source)
		if serrS != nil {
			return tools.Deny(ctx, "move_file", source, serrS.Error()), nil
		}
		scT, serrT := tools.ScopeWrite(ctx, target)
		if serrT != nil {
			tools.RecordOperation(ctx, deps.Store, sessionID, "move_file", target, "denied", serrT.Error(), params)
			return tools.Deny(ctx, "move_file", target, serrT.Error()), nil
		}
		src, dst := scS.Key, scT.Key

		// 源写授权：移动改的是源文件的键（归属矩阵判定）
		if denied, reason := tools.CanFile(ctx, deps.Store, src, true); denied {
			tools.RecordOperation(ctx, deps.Store, sessionID, "move_file", src, "denied", reason, params)
			return tools.Deny(ctx, "move_file", src, reason), nil
		}

		// 键改走管理机（intake 域 Move：uuid 不变零重分析，ext 变则物理位
		// 随派生规则 rename；谱系 moved_from 记原键）
		if deps.Manager == nil {
			return tools.ResultError("manager not wired"), nil
		}
		receipt, err := deps.Manager.Move(ctx, src, dst)
		if err != nil {
			slog.Error("move_file failed", "from", src, "to", dst, "session", sessionID, "error", err, "duration", time.Since(start).String())
			tools.RecordOperation(ctx, deps.Store, sessionID, "move_file", src, "failed", err.Error(), params)
			return tools.ResultError(err.Error()), nil
		}

		// 编排机事件仅记账（KindMove 执行器早退：描述/索引零触发）
		if deps.Orch != nil {
			deps.Orch.Submit(core.Event{
				Kind:      core.KindMove,
				Path:      dst,
				SessionID: sessionID,
				Actor:     tools.Actor(ctx),
			})
		}

		slog.Info("move_file ok", "from", src, "to", dst, "uuid", receipt.UUID, "storage_move", receipt.StorageMove, "session", sessionID, "duration", time.Since(start).String())
		tools.RecordOperation(ctx, deps.Store, sessionID, "move_file", dst, "success", "", params)
		return tools.Result(map[string]any{
			"success":      true,
			"uuid":         receipt.UUID,
			"from":         receipt.From,
			"to":           receipt.To,
			"storage_move": receipt.StorageMove,
			"message":      "Successfully moved " + receipt.From + " to " + receipt.To,
		}), nil
	})
}
