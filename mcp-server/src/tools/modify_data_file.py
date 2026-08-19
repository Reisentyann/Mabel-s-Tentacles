import os
from pathlib import Path
from typing import Dict, Any

BASE_DIR = Path('data').resolve()

async def modify_data_file_tool(file_path: str, content: str, mode: str = "append") -> Dict[str, Any]:
    """Modifies an existing file in the data directory.
    
    Args:
        file_path: The path to the file relative to the data directory.
        content: The content to write or append.
        mode: 'append' to add content to the end of the file, 'overwrite' to replace the whole file.
    """
    try:
        if len(content.encode('utf-8')) > 5 * 1024 * 1024:
            return {"success": False, "message": "Security Error: File content exceeds 5MB limit."}

        clean_path = file_path.lstrip('/\\')
        if ':' in clean_path:
            return {"success": False, "message": "Security Error: Path cannot contain drive letters."}

        target_path = (BASE_DIR / clean_path).resolve()

        if not target_path.is_relative_to(BASE_DIR):
            return {"success": False, "message": "Security Error: Directory traversal detected and blocked."}
            
        if not target_path.exists():
            return {"success": False, "message": f"Error: File '{file_path}' does not exist. Cannot modify a non-existent file."}
            
        if not target_path.is_file():
            return {"success": False, "message": f"Error: Path '{file_path}' is not a file."}

        if mode == "append":
            with target_path.open("a", encoding="utf-8") as f:
                f.write(content)
        elif mode == "overwrite":
            target_path.write_text(content, encoding='utf-8')
        else:
            return {"success": False, "message": f"Error: Invalid mode '{mode}'. Must be 'append' or 'overwrite'."}
            
        return {"success": True, "message": f"Successfully modified {target_path} in {mode} mode"}
        
    except Exception as e:
        return {"success": False, "message": f"Modification failed: {str(e)}"}
