// 文件：mcp-server-go/internal/tools/directories/directories.go —— UUID 目录 MCP 接口：列出、新建子目录和同名文件写入
// 修改：2026-10-08（日期由 fresh-header.ps1 刷新）

package directories

import (
	"context"
	"github.com/Reisentyann/Mabel-s-Tentacles/common"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"strings"
)

func init() { tools.Register(register) }

func register(s *server.MCPServer, deps tools.Deps) {
	for _, op := range []string{"list_directory", "create_child_directory", "write_directory_file"} {
		op := op
		tool := mcp.NewTool(op, mcp.WithDescription("Operate on a directory by UUID. Same-name uploads are independent files; use returned UUID to retrieve."), mcp.WithString("directory_uuid", mcp.Required()), mcp.WithString("name"), mcp.WithString("content"))
		s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if deps.Manager == nil {
				return tools.ResultError("manager not wired"), nil
			}
			id, err := req.RequireString("directory_uuid")
			if err != nil {
				return tools.ResultError(err.Error()), nil
			}
			ref, err := deps.Manager.LocateDirectory(ctx, id)
			if err != nil {
				return tools.ResultError(err.Error()), nil
			}
			p := tools.Principal(ctx)
			if p == nil || (!p.IsAdmin() && ref.Path != "~"+p.Name && !strings.HasPrefix(ref.Path, "~"+p.Name+"/")) {
				return tools.Deny(ctx, op, ref.Path, "directory belongs to another user"), nil
			}
			if op == "list_directory" {
				items, err := deps.Manager.ListDirectory(ctx, id)
				if err != nil {
					return tools.ResultError(err.Error()), nil
				}
				visible := items[:0]
				for _, item := range items {
					if item.Kind == "file" {
						ref, err := deps.Manager.Locate(ctx, item.UUID)
						if err != nil {
							continue
						}
						if denied, _ := tools.CanFile(ctx, deps.Store, ref.Path, false); denied {
							continue
						}
					}
					visible = append(visible, item)
				}
				return tools.Result(map[string]any{"success": true, "entries": visible}), nil
			}
			name, err := req.RequireString("name")
			if err != nil {
				return tools.ResultError(err.Error()), nil
			}
			if op == "create_child_directory" {
				r, err := deps.Manager.CreateChildDirectory(ctx, id, name)
				if err != nil {
					return tools.ResultError(err.Error()), nil
				}
				return tools.Result(map[string]any{"success": true, "uuid": r.UUID, "path": r.Path}), nil
			}
			content, err := req.RequireString("content")
			if err != nil {
				return tools.ResultError(err.Error()), nil
			}
			r, err := deps.Manager.WriteInDirectory(ctx, id, name, content)
			if err != nil {
				return tools.ResultError(err.Error()), nil
			}
			if deps.Orch != nil {
				deps.Orch.Submit(core.Event{Kind: core.KindWrite, Path: r.LogicPath, Actor: tools.Actor(ctx), SessionID: tools.SessionID(ctx), Agent: &core.AgentMeta{Title: common.StrPtr(name)}})
			}
			return tools.Result(map[string]any{"success": true, "uuid": r.UUID, "name": name}), nil
		})
	}
}
