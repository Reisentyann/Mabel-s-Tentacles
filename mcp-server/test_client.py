import asyncio
from mcp.client.sse import sse_client
from mcp.client.session import ClientSession

async def main():
    print("Connecting to SSE...")
    try:
        async with sse_client("http://localhost:8001/sse") as (read_stream, write_stream):
            print("SSE connection established")
            async with ClientSession(read_stream, write_stream) as session:
                await session.initialize()
                print("Session initialized")
                tools = await session.list_tools()
                print("Tools:", tools)
    except Exception as e:
        print("Error:", e)

asyncio.run(main())