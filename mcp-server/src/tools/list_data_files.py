import os
from pathlib import Path
from typing import Dict, Any, List

BASE_DIR = Path('data').resolve()

async def list_data_files_tool() -> Dict[str, Any]:
    """Lists all files in the data directory."""
    try:
        if not BASE_DIR.exists():
            BASE_DIR.mkdir(parents=True, exist_ok=True)
            
        files = []
        for file_path in BASE_DIR.rglob('*'):
            if file_path.is_file():
                files.append(file_path.relative_to(BASE_DIR).as_posix())
                
        return {"success": True, "files": files}
    except Exception as e:
        return {"success": False, "message": f"Failed to list files: {str(e)}"}
