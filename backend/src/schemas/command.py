from pydantic import BaseModel
from typing import Optional, Any
from datetime import datetime

class CommandResponse(BaseModel):
    id: int
    user_id: int
    source: str
    command_text: str
    command_type: str
    status: str
    result: Optional[str] = None
    error_message: Optional[str] = None
    exit_code: Optional[int] = None
    environment: dict = {}
    created_at: datetime
    finished_at: Optional[datetime] = None

    class Config:
        from_attributes = True

class PaginatedCommandsResponse(BaseModel):
    items: list[CommandResponse]
    total: int
    page: int
    size: int