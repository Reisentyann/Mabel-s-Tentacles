from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select
from datetime import datetime

from src.database import get_db
from src.models import User, TokenBlacklist
from src.schemas.auth import UserCreate, UserResponse, Token, LoginRequest, RefreshRequest
from src.services.auth_service import (
    get_password_hash,
    verify_password,
    create_access_token,
    create_refresh_token,
    decode_token
)

router = APIRouter()

@router.post("/register", response_model=UserResponse)
async def register(user: UserCreate, db: AsyncSession = Depends(get_db)):
    print(f"[DEBUG] Registration attempt for username: {user.username}")
    try:
        result = await db.execute(select(User).where(User.username == user.username))
        existing_user = result.scalars().first()
        print(f"[DEBUG] Existing user check result: {existing_user}")
        
        if existing_user:
            print("[DEBUG] Error: Username already registered")
            raise HTTPException(status_code=400, detail="Username already registered")
            
        print("[DEBUG] Generating password hash and creating user record")
        db_user = User(
            username=user.username,
            email=user.email,
            password_hash=get_password_hash(user.password)
        )
        db.add(db_user)
        print("[DEBUG] Adding user to DB session")
        await db.commit()
        print("[DEBUG] DB commit successful")
        await db.refresh(db_user)
        print(f"[DEBUG] Registration successful for user ID: {db_user.id}")
        return db_user
    except Exception as e:
        print(f"[DEBUG] Exception caught during registration: {str(e)}")
        raise

@router.post("/login", response_model=Token)
async def login(req: LoginRequest, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(User).where(User.username == req.username))
    user = result.scalars().first()
    
    if not user or not verify_password(req.password, user.password_hash):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
            headers={"WWW-Authenticate": "Bearer"},
        )
        
    access_token = create_access_token(data={"sub": user.username, "user_id": user.id})
    refresh_token = create_refresh_token(data={"sub": user.username, "user_id": user.id})
    
    return {"access_token": access_token, "refresh_token": refresh_token, "token_type": "bearer"}

@router.post("/refresh", response_model=Token)
async def refresh(req: RefreshRequest, db: AsyncSession = Depends(get_db)):
    try:
        payload = decode_token(req.refresh_token)
        jti = payload.get("jti")
        
        # Check if refresh token is blacklisted
        result = await db.execute(select(TokenBlacklist).where(TokenBlacklist.token_jti == jti))
        if result.scalars().first():
            raise HTTPException(status_code=401, detail="Token has been revoked")
            
        # Blacklist old refresh token
        exp = datetime.fromtimestamp(payload.get("exp"))
        blacklist_entry = TokenBlacklist(token_jti=jti, expires_at=exp)
        db.add(blacklist_entry)
        
        # Issue new tokens
        access_token = create_access_token(data={"sub": payload.get("sub"), "user_id": payload.get("user_id")})
        new_refresh_token = create_refresh_token(data={"sub": payload.get("sub"), "user_id": payload.get("user_id")})
        
        await db.commit()
        
        return {"access_token": access_token, "refresh_token": new_refresh_token, "token_type": "bearer"}
    except Exception as e:
        raise HTTPException(status_code=401, detail="Invalid refresh token")

@router.post("/logout")
async def logout(req: RefreshRequest, db: AsyncSession = Depends(get_db)):
    try:
        payload = decode_token(req.refresh_token)
        jti = payload.get("jti")
        exp = datetime.fromtimestamp(payload.get("exp"))
        
        blacklist_entry = TokenBlacklist(token_jti=jti, expires_at=exp)
        db.add(blacklist_entry)
        await db.commit()
        return {"message": "Successfully logged out"}
    except:
        return {"message": "Successfully logged out"} # Ignore errors on logout