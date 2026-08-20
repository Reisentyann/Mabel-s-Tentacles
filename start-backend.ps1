# Mabel's Tentacles - Backend Launcher (PowerShell 7)
$ErrorActionPreference = "Stop"

$Root = $PSScriptRoot
$DBPort = "54322"
$BackendPort = "18000"

Write-Host "=============================================="
Write-Host "  Mabel's Tentacles - Backend Launcher"
Write-Host "=============================================="
Write-Host ""

# 1. Python
if (-not (Get-Command python -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] Python not found. Install Python 3.11+ first." -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

# 2. Dependencies
Write-Host "[1/4] Installing dependencies..."
pip install -r "$Root\backend\requirements.txt" watchfiles -i https://mirrors.aliyun.com/pypi/simple/ -q
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Dependency install failed." -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}
Write-Host "      Dependencies ready."

# 3. Database
Write-Host "[2/4] Checking database..."
docker info *> $null
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Docker is not running. Start Docker Desktop first." -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

$existing = docker ps -a --filter "name=agent_postgres" --format "{{.Names}}" 2>$null
if ($existing -match "agent_postgres") {
    Write-Host "      Starting existing container..."
    docker start agent_postgres *> $null
} else {
    Write-Host "      Creating agent_postgres container..."
    docker run -d --name agent_postgres `
        -e POSTGRES_USER=postgres `
        -e POSTGRES_PASSWORD=postgres `
        -e POSTGRES_DB=agent_db `
        -p "${DBPort}:5432" `
        -v "${Root}\database\init.sql:/docker-entrypoint-initdb.d/init.sql:ro" `
        postgres:18 *> $null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERROR] Failed to create database container. Port $DBPort may be in use." -ForegroundColor Red
        Read-Host "Press Enter to exit"
        exit 1
    }
}

# 4. Wait for database
Write-Host "[3/4] Waiting for database..."
while ($true) {
    docker exec agent_postgres pg_isready -U postgres *> $null
    if ($LASTEXITCODE -eq 0) { break }
    Start-Sleep -Seconds 2
}
Write-Host "      Database ready."

# 5. Backend
Write-Host "[4/4] Starting backend..."
Write-Host ""
Write-Host "  API:       http://localhost:$BackendPort"
Write-Host "  Docs:      http://localhost:$BackendPort/docs"
Write-Host "  Data dir:  $Root\data"
Write-Host "  Stop:      Ctrl+C"
Write-Host ""

$env:DATABASE_URL = "postgresql+asyncpg://postgres:postgres@localhost:${DBPort}/agent_db"
$env:DATA_DIR = "$Root\data"
Set-Location "$Root\backend"
uvicorn src.main:app --reload --host 0.0.0.0 --port $BackendPort
