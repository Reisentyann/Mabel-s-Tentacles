from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

app = FastAPI(title="Agent API", version="0.1.0")

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=[
        "http://localhost:18080",
        "http://127.0.0.1:18080",
        "http://localhost:5173",  # default vite port fallback
        "http://127.0.0.1:5173",
    ],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

@app.get("/health")
async def health_check():
    return {"status": "ok"}

from src.routes import auth, commands

app.include_router(auth.router, prefix="/api/auth", tags=["auth"])
app.include_router(commands.router, prefix="/api/commands", tags=["commands"])
# app.include_router(email.router, prefix="/api/email", tags=["email"])
