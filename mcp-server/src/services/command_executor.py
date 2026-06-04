import asyncio
from typing import Tuple

class CommandExecutor:
    @staticmethod
    async def execute(command: str, timeout: int = 60) -> Tuple[int, str, str]:
        """
        Executes a shell command and returns (exit_code, stdout, stderr).
        """
        try:
            process = await asyncio.create_subprocess_shell(
                command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            
            try:
                stdout, stderr = await asyncio.wait_for(process.communicate(), timeout=timeout)
                exit_code = process.returncode
                return exit_code, stdout.decode('utf-8', errors='replace'), stderr.decode('utf-8', errors='replace')
            except asyncio.TimeoutError:
                process.kill()
                return -1, "", "Command timed out"
                
        except Exception as e:
            return -1, "", str(e)