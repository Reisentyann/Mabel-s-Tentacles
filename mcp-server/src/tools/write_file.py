import os
from typing import Dict, Any

async def write_file_tool(file_path: str, content: str) -> Dict[str, Any]:
    """Writes content to a file."""
    try:
        # Ensure the directory exists
        os.makedirs(os.path.dirname(file_path), exist_ok=True)
        
        with open(file_path, 'w', encoding='utf-8') as f:
            f.write(content)
            
        return {"success": True, "message": f"Successfully wrote to {file_path}"}
    except Exception as e:
        return {"success": False, "message": str(e)}