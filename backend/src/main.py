from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from contextlib import asynccontextmanager
from src.config import settings

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Auto-create admin user on startup
    from src.database import AsyncSessionLocal
    from src.models import User
    from src.services.auth_service import get_password_hash
    from sqlalchemy.future import select

    try:
        async with AsyncSessionLocal() as session:
            result = await session.execute(select(User).where(User.username == settings.ADMIN_USERNAME))
            admin_user = result.scalars().first()
            if not admin_user:
                print(f"[INFO] Creating default admin user: {settings.ADMIN_USERNAME}")
                new_admin = User(
                    username=settings.ADMIN_USERNAME,
                    email="admin@example.com",
                    password_hash=get_password_hash(settings.ADMIN_PASSWORD)
                )
                session.add(new_admin)
                await session.commit()
    except Exception as e:
        print(f"[WARNING] Could not create admin user during startup: {e}")
        
    yield

app = FastAPI(title="Agent API", version="0.1.0", lifespan=lifespan)

# Setup CORS origins
origins = [
    "http://localhost:18080",
    "http://127.0.0.1:18080",
    "http://localhost:5173",
    "http://127.0.0.1:5173",
]

if settings.CORS_ORIGINS:
    origins.extend([origin.strip() for origin in settings.CORS_ORIGINS.split(",") if origin.strip()])

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

@app.get("/health")
async def health_check():
    return {"status": "ok"}

from src.routes import auth, commands, files

app.include_router(auth.router, prefix="/api/auth", tags=["auth"])
app.include_router(commands.router, prefix="/api/commands", tags=["commands"])
app.include_router(files.router, prefix="/api/files", tags=["files"])
# app.include_router(email.router, prefix="/api/email", tags=["email"])
