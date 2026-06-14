import os
from pathlib import Path
from typing import Dict, Any

# 1. 明确定义并解析出目标目录的“绝对路径”
# BASE_DIR 将变成类似: /root/my_project/data 或 C:\my_project\data
BASE_DIR = Path('data').resolve()

async def write_file_tool(file_path: str, content: str) -> Dict[str, Any]:
    """Writes content to a file safely."""
    try:
        # 【安全点1】限制文件大小，防止撑爆硬盘/内存（这里限制为 5MB）
        if len(content.encode('utf-8')) > 5 * 1024 * 1024:
            return {"success": False, "message": "Security Error: File content exceeds 5MB limit."}

        # 剥离可能存在的绝对路径前缀和反斜杠
        clean_path = file_path.lstrip('/\\')
        
        # 【安全点2】防止 Windows 下的盘符攻击 (禁止输入类似 C: 的内容)
        if ':' in clean_path:
            return {"success": False, "message": "Security Error: Path cannot contain drive letters."}

        # 【安全点3】使用 resolve() 计算出最终的绝对路径
        # resolve() 会自动消除掉所有的 '..'，并且会看透所有的符号链接(快捷方式)
        target_path = (BASE_DIR / clean_path).resolve()

        # 【安全点4】终极防御：判断最终路径是否仍然在 BASE_DIR 里面！
        # 无论 AI 用什么花招，只要最终指向的文件不在 data/ 目录下，统统拦截
        if not target_path.is_relative_to(BASE_DIR):
            return {"success": False, "message": "Security Error: Directory traversal detected and blocked."}
        
        # 确保目录存在
        target_path.parent.mkdir(parents=True, exist_ok=True)
        
        # 写入文件 (使用 pathlib 的 write_text 更加简洁)
        target_path.write_text(content, encoding='utf-8')
            
        return {"success": True, "message": f"Successfully wrote to {target_path}"}
        
    except Exception as e:
        # 不要向客户端/AI 暴露具体的系统级错误堆栈，只返回基础信息
        return {"success": False, "message": f"Write failed: {str(e)}"}