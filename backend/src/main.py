from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from src.config import settings

app = FastAPI(title="Agent API", version="0.1.0")

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
