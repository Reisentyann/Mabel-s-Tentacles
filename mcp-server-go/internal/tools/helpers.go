package tools

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/store"
)

func jsonResult(v map[string]any) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	return mcp.NewToolResultText(string(b))
}

func jsonError(message string) *mcp.CallToolResult {
	return jsonResult(map[string]any{"success": false, "message": message})
}

func sessionIDFromContext(ctx context.Context) string {
	if s := server.ClientSessionFromContext(ctx); s != nil {
		return s.SessionID()
	}
	return ""
}

func recordOperation(ctx context.Context, st *store.Store, sessionID, tool, filePath, status, errMsg string, params map[string]any) {
	if st == nil {
		return
	}
	if err := st.RecordOperation(ctx, sessionID, tool, filePath, status, errMsg, params); err != nil {
		slog.Warn("record operation failed", "tool", tool, "error", err)
	}
}
