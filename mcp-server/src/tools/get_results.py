from src.services.db import db
from typing import Dict, Any, List

async def get_results_tool(user_id: int, limit: int = 10) -> Dict[str, Any]:
    """Fetches command results for a user."""
    try:
        query = """
            SELECT id, command_text, status, result, error_message, exit_code, created_at, finished_at
            FROM commands
            WHERE user_id = $1
            ORDER BY created_at DESC
            LIMIT $2
        """
        records = await db.fetch(query, user_id, limit)
        
        results = []
        for record in records:
            results.append({
                "id": record["id"],
                "command_text": record["command_text"],
                "status": record["status"],
                "result": record["result"],
                "error_message": record["error_message"],
                "exit_code": record["exit_code"],
                "created_at": record["created_at"].isoformat() if record["created_at"] else None,
                "finished_at": record["finished_at"].isoformat() if record["finished_at"] else None
            })
            
        return {"success": True, "data": results}
    except Exception as e:
        return {"success": False, "message": str(e)}