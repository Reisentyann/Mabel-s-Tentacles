import os
from typing import List
from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import FileResponse
from src.middleware.auth import get_current_user
from src.models import User

router = APIRouter()

# Data directory path based on docker-compose mount
DATA_DIR = "/app/data"

@router.get("/")
async def list_files(current_user: User = Depends(get_current_user)):
    """List all files in the data directory."""
    if not os.path.exists(DATA_DIR):
        return {"files": []}
    
    files_info = []
    try:
        for filename in os.listdir(DATA_DIR):
            file_path = os.path.join(DATA_DIR, filename)
            if os.path.isfile(file_path):
                stat = os.stat(file_path)
                files_info.append({
                    "name": filename,
                    "size": stat.st_size,
                    "modified_at": stat.st_mtime
                })
        # Sort by modification time, newest first
        files_info.sort(key=lambda x: x["modified_at"], reverse=True)
        return {"files": files_info}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to read directory: {str(e)}")

@router.get("/download/{filename}")
async def download_file(filename: str, current_user: User = Depends(get_current_user)):
    """Download a specific file from the data directory."""
    # Security check: prevent directory traversal
    if ".." in filename or "/" in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="Invalid filename")
        
    file_path = os.path.join(DATA_DIR, filename)
    
    if not os.path.exists(file_path) or not os.path.isfile(file_path):
        raise HTTPException(status_code=404, detail="File not found")
        
    return FileResponse(
        path=file_path,
        filename=filename,
        media_type="application/octet-stream"
    )
