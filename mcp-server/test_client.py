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
                print("Session initialized\n")
                
                print("--- 1. Testing list_data_files ---")
                res = await session.call_tool("list_data_files")
                print("Result:", res)
                
                print("\n--- 2. Testing modify_data_file (append) ---")
                res2 = await session.call_tool("modify_data_file", arguments={
                    "file_path": "test.txt",
                    "content": "\nfrom mcp test client",
                    "mode": "append"
                })
                print("Result:", res2)
                
                print("\n--- 3. Testing modify_data_file (overwrite) ---")
                res3 = await session.call_tool("modify_data_file", arguments={
                    "file_path": "test.txt",
                    "content": "overwritten from mcp test client",
                    "mode": "overwrite"
                })
                print("Result:", res3)
                
    except Exception as e:
        import traceback
        traceback.print_exc()
        print("Error:", e)

if __name__ == "__main__":
    asyncio.run(main())