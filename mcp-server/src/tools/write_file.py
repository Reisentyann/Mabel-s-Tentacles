import os
from typing import Dict, Any

async def write_file_tool(file_path: str, content: str) -> Dict[str, Any]:
    """Writes content to a file."""
    try:
        # Ensure all files are written to the 'data' directory
        if os.path.isabs(file_path):
            file_path = file_path.lstrip('/\\')
            
        # Prevent directory traversal
        if '..' in file_path:
            return {"success": False, "message": "Directory traversal is not allowed"}
            
        final_path = os.path.join('data', file_path)
        
        # Ensure the directory exists
        dirname = os.path.dirname(final_path)
        if dirname:
            os.makedirs(dirname, exist_ok=True)
        
        with open(final_path, 'w', encoding='utf-8') as f:
            f.write(content)
            
        return {"success": True, "message": f"Successfully wrote to {final_path}"}
    except Exception as e:
        return {"success": False, "message": str(e)}