import asyncio
import os
import sys
import json
from contextlib import asynccontextmanager
from mcp.server.fastmcp import FastMCP

from src.services.db import db
from src.tools.execute_command import execute_command_tool
from src.tools.get_results import get_results_tool
from src.tools.write_file import write_file_tool

@asynccontextmanager
async def server_lifespan(server: FastMCP):
    # Initialize DB connection on startup
    await db.connect()
    yield
    # Disconnect on shutdown
    await db.disconnect()

port = int(os.getenv("PORT", "8001"))

# Create MCP server instance
mcp = FastMCP("agent-mcp-server", host="0.0.0.0", port=port, lifespan=server_lifespan)

@mcp.tool()
async def execute_command(command: str, user_id: int) -> str:
    """Execute a shell command. Use this when the user's intent is to run a command or perform an action on the system."""
    result = await execute_command_tool(command, user_id)
    return json.dumps(result)

@mcp.tool()
async def get_results(user_id: int, limit: int = 10) -> str:
    """Get the history and results of previously executed commands for a user."""
    result = await get_results_tool(user_id, limit)
    return json.dumps(result)

@mcp.tool()
async def write_file(file_path: str, content: str) -> str:
    """Write generated content to a file. Use this when the user wants to generate code, write an article, or create a file."""
    result = await write_file_tool(file_path, content)
    return json.dumps(result)

def main():
    """Run the FastMCP server."""
    mcp.run("sse")

if __name__ == "__main__":
    main()
