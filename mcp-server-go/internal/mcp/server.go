// 文件：mcp-server-go/internal/mcp/server.go —— MCP 服务器装配：SSE 端点 + 工具注册（tools/all）
// 修改：2026-10-14（日期由 fresh-header.ps1 刷新）

package mcp

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/Reisentyann/Mabel-s-Tentacles/manager-go"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/core"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/config"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/repo"
	"github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools"
	_ "github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go/internal/tools/all"
)

// New 装配 MCP 服务器。orch 为编排机（写路径事件 + Describe 入口；
// 三机串联的注入点，见 core 包注释）。
func New(cfg *config.Config, st repo.Store, mgr *manager.Manager, orch *core.Orchestrator) *server.MCPServer {
	s := server.NewMCPServer(
		"mabel-mcp-server",
		"0.2.0",
		server.WithRecovery(),
		server.WithInstructions(serverInstructions),
	)

	tools.RegisterAll(s, tools.Deps{Cfg: cfg, Store: st, Manager: mgr, Orch: orch})
	registerResources(s, orch)
	registerPrompts(s)

	return s
}

const serverInstructions = `Mabel's Tentacles — Deterministic File Manager, Describer & Indexer MCP Server.

[TOOL BOUNDARY & WORKFLOW MATRIX]
1. UPLOAD & CREATE (New files only; existing paths are rejected):
   - Text / Code / Markdown -> use 'write_file' (pass file_path, content, and inline title/description/tags/attributes).
   - Images / Binary / Archives (or Base64 / Data URLs / Local Host Files) -> use 'upload_file' (pass file_path + content_base64 OR source_path, plus inline title/description/tags/attributes).
   - Every uploaded/written file is automatically analyzed by the Describer Engine (producing 90+ read-only 'cod-*' facts like image resolution, palette, EXIF, text lines, language) and indexed in memory.

2. EDIT EXISTING FILES (Never use write_file to overwrite):
   - Small targeted edits -> use 'patch_file' (unique old_string -> new_string replacement).
   - Append or full-file replace -> use 'modify_file' (mode='append' or 'overwrite').
   - Rename / Move -> use 'move_file' (preserves UUID and metadata).
   - Duplicate -> use 'copy_file' (tracks copied_from lineage).
   - Delete -> use 'delete_file' (soft delete & removes from index).

3. DESCRIBE vs ANALYZE (Metadata & Fact Tracks):
   - 'describe_file': Annotate human/LLM semantic fields (title, description, tags, visibility, and 'llm-*' / 'sp-llm-*' attributes such as llm-semantic-type, llm-characters, llm-style, llm-summary). NEVER pass 'cod-*' keys here.
   - 'analyze_file': Re-run the deterministic engine on disk bytes to refresh 'cod-*' facts (only needed after out-of-band shell changes or engine upgrades).

4. SEARCH & INSPECT (2-Step Precision Retrieval):
   - Step 1 (Discover): Call 'list_index_fields' (or read resource 'mabel://catalog/fields') ONCE per session to learn available 'cod-*'/'llm-*' fields, value ranges, and calibration benchmarks ('bench').
   - Step 2 (Query): Call 'search_files' with boolean condition trees (eq, in, gt, lt, range, ne, exists, contains) and optional order_by.
   - Browse / Inspect: Use 'list_files' for paginated browsing, 'get_file_info' for full 90+ attributes of a single file, and 'read_file' to read text content.

5. DIRECTORIES (Path vs UUID):
   - Path-based: 'create_directory' creates a logical folder path (e.g. 'notes/2026').
   - UUID-based: 'list_directory', 'create_child_directory', 'write_directory_file' operate by stable directory_uuid and allow duplicate display names.`
