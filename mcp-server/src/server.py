import asyncio
import os
import sys
import json
from mcp.server import Server, NotificationOptions
from mcp.server.models import InitializationOptions
import mcp.server.stdio
from mcp.types import Tool, TextContent, CallToolRequest

from src.services.db import db
from src.tools.execute_command import execute_command_tool
from src.tools.get_results import get_results_tool
from src.tools.write_file import write_file_tool

# Create MCP server instance
server = Server("agent-mcp-server")

@server.list_tools()
async def handle_list_tools() -> list[Tool]:
    """List available tools."""
    return [
        Tool(
            name="execute_command",
            description="Execute a shell command. Use this when the user's intent is to run a command or perform an action on the system.",
            inputSchema={
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "The shell command to execute."
                    },
                    "user_id": {
                        "type": "integer",
                        "description": "The ID of the user requesting the execution."
                    }
                },
                "required": ["command", "user_id"]
            }
        ),
        Tool(
            name="get_results",
            description="Get the history and results of previously executed commands for a user.",
            inputSchema={
                "type": "object",
                "properties": {
                    "user_id": {
                        "type": "integer",
                        "description": "The ID of the user whose command results are being requested."
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum number of results to return (default 10)."
                    }
                },
                "required": ["user_id"]
            }
        ),
        Tool(
            name="write_file",
            description="Write generated content to a file. Use this when the user wants to generate code, write an article, or create a file.",
            inputSchema={
                "type": "object",
                "properties": {
                    "file_path": {
                        "type": "string",
                        "description": "The absolute or relative path where the file should be written."
                    },
                    "content": {
                        "type": "string",
                        "description": "The content to write into the file."
                    }
                },
                "required": ["file_path", "content"]
            }
        )
    ]

@server.call_tool()
async def handle_call_tool(name: str, arguments: dict | None) -> list[TextContent]:
    """Handle tool execution requests."""
    if not arguments:
        raise ValueError("Missing arguments")

    if name == "execute_command":
        command = arguments.get("command")
        user_id = arguments.get("user_id")
        
        if not command:
            raise ValueError("Missing command")
        if not user_id:
            raise ValueError("Missing user_id")
            
        result = await execute_command_tool(command, user_id)
        return [TextContent(type="text", text=json.dumps(result))]
        
    elif name == "get_results":
        user_id = arguments.get("user_id")
        limit = arguments.get("limit", 10)
        
        if not user_id:
            raise ValueError("Missing user_id")
            
        result = await get_results_tool(user_id, limit)
        return [TextContent(type="text", text=json.dumps(result))]
        
    elif name == "write_file":
        file_path = arguments.get("file_path")
        content = arguments.get("content")
        
        if not file_path:
            raise ValueError("Missing file_path")
        if not content:
            raise ValueError("Missing content")
            
        result = await write_file_tool(file_path, content)
        return [TextContent(type="text", text=json.dumps(result))]
        
    raise ValueError(f"Unknown tool: {name}")

async def main():
    """Run the MCP server."""
    # Initialize DB connection
    await db.connect()
    
    # Start server with stdio transport
    async with mcp.server.stdio.stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            InitializationOptions(
                server_name="agent-mcp-server",
                server_version="0.1.0",
                capabilities=server.get_capabilities(
                    notification_options=NotificationOptions(),
                    experimental_capabilities={},
                ),
            ),
        )

if __name__ == "__main__":
    asyncio.run(main())