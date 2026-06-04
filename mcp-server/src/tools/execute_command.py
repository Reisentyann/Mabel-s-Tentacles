from src.services.command_executor import CommandExecutor
from src.services.db import db
from typing import Dict, Any

async def execute_command_tool(command: str, user_id: int) -> Dict[str, Any]:
    """Executes a shell command and logs it to the database."""
    
    # 1. Insert pending command record
    try:
        insert_query = """
            INSERT INTO commands (user_id, command_text, status)
            VALUES ($1, $2, 'running')
            RETURNING id
        """
        record = await db.fetchrow(insert_query, user_id, command)
        command_id = record['id']
    except Exception as e:
        return {"success": False, "message": f"Database error: {str(e)}"}

    # 2. Execute command
    exit_code, stdout, stderr = await CommandExecutor.execute(command)

    # 3. Update command record with result
    try:
        status = 'done' if exit_code == 0 else 'error'
        update_query = """
            UPDATE commands
            SET status = $1, result = $2, error_message = $3, exit_code = $4, finished_at = NOW()
            WHERE id = $5
        """
        await db.execute(update_query, status, stdout, stderr, exit_code, command_id)
        
        return {
            "success": exit_code == 0,
            "exit_code": exit_code,
            "stdout": stdout,
            "stderr": stderr
        }
    except Exception as e:
        return {"success": False, "message": f"Failed to update database: {str(e)}"}