from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select
from sqlalchemy import func

from src.database import get_db
from src.models import Command, User
from src.schemas.command import CommandResponse, PaginatedCommandsResponse
from src.middleware.auth import get_current_user

router = APIRouter()

@router.get("/", response_model=PaginatedCommandsResponse)
async def get_commands(
    page: int = Query(1, ge=1),
    size: int = Query(10, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    offset = (page - 1) * size
    
    # Get total count
    query_count = select(func.count(Command.id)).where(Command.user_id == current_user.id)
    result_count = await db.execute(query_count)
    total = result_count.scalar()
    
    # Get items
    query_items = select(Command).where(Command.user_id == current_user.id).order_by(Command.created_at.desc()).offset(offset).limit(size)
    result_items = await db.execute(query_items)
    items = result_items.scalars().all()
    
    return {
        "items": items,
        "total": total,
        "page": page,
        "size": size
    }

@router.get("/{command_id}", response_model=CommandResponse)
async def get_command(
    command_id: int,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    query = select(Command).where(Command.id == command_id, Command.user_id == current_user.id)
    result = await db.execute(query)
    command = result.scalars().first()
    
    if not command:
        raise HTTPException(status_code=404, detail="Command not found")
        
    return command