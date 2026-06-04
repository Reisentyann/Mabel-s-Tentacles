from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select
from src.database import get_db
from src.models import User, TokenBlacklist
from src.services.auth_service import decode_token

security = HTTPBearer()

async def get_current_user(credentials: HTTPAuthorizationCredentials = Depends(security), db: AsyncSession = Depends(get_db)) -> User:
    token = credentials.credentials
    try:
        payload = decode_token(token)
        username: str = payload.get("sub")
        jti: str = payload.get("jti")
        if username is None:
            raise HTTPException(status_code=401, detail="Invalid authentication credentials")
            
        # Check if token is blacklisted
        result = await db.execute(select(TokenBlacklist).where(TokenBlacklist.token_jti == jti))
        if result.scalars().first():
            raise HTTPException(status_code=401, detail="Token has been revoked")
            
    except Exception:
        raise HTTPException(status_code=401, detail="Invalid authentication credentials")
        
    result = await db.execute(select(User).where(User.username == username))
    user = result.scalars().first()
    if user is None:
        raise HTTPException(status_code=401, detail="User not found")
    return user