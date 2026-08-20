# start-dev.ps1 — 本地联调：编译并启动 Go 后端(8080) + 前端 dev(5173)
# 用法：在 PowerShell 里执行  .\start-dev.ps1
# 停止：Ctrl+C（会自动停掉后端）

$ErrorActionPreference = "Stop"

# =====================================================================
# 路径（自动定位到项目根目录，无需手改）
# =====================================================================
$Root        = $PSScriptRoot                            # 本脚本所在目录 = 项目根
$BackendDir  = Join-Path $Root "mcp-server-go"          # Go 后端目录
$FrontendDir = Join-Path $Root "frontend"               # 前端目录
$BackendExe  = Join-Path $BackendDir "bin\server.exe"   # 后端可执行文件

foreach ($d in @($BackendDir, $FrontendDir)) {
    if (-not (Test-Path -LiteralPath $d)) {
        throw "目录不存在：$d （请确认在项目根目录运行本脚本）"
    }
}

# =====================================================================
# 读取项目根 .env（数据库 / 管理员 / JWT 配置的唯一来源）
# =====================================================================
$envFile = Join-Path $Root ".env"
if (Test-Path -LiteralPath $envFile) {
    Get-Content -LiteralPath $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
            [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim(), "Process")
        }
    }
}

# =====================================================================
# 把 .env 里的相对路径解析成绝对路径（以项目根 $Root 为基准，避免 CWD 歧义）
# =====================================================================
foreach ($key in @("DATA_DIR", "LOG_FILE")) {
    $val = [Environment]::GetEnvironmentVariable($key, "Process")
    if ($val -and -not [System.IO.Path]::IsPathRooted($val)) {
        [Environment]::SetEnvironmentVariable($key, (Join-Path $Root $val), "Process")
    }
}
# 兜底：.env 没配时用项目根下的默认值（绝对路径）
if (-not $env:DATA_DIR) { $env:DATA_DIR = Join-Path $Root "data" }
if (-not $env:LOG_FILE) { $env:LOG_FILE = Join-Path $Root "logs\server.log" }

# =====================================================================
# 端口
# =====================================================================
$BackendPort  = 8080
$FrontendPort = 5173

# =====================================================================
# 数据库连接（本地开发 = 连 Docker 映射到 54322 的 postgres）
# =====================================================================
# 优先用环境变量 DATABASE_URL；否则按 .env 里的 POSTGRES_* 拼接。
$DB_URL = $env:DATABASE_URL
if (-not $DB_URL) {
    $dbHost = if ($env:POSTGRES_HOST)     { $env:POSTGRES_HOST }     else { "localhost" }
    $dbPort = if ($env:POSTGRES_PORT)     { $env:POSTGRES_PORT }     else { "54322" }
    $dbUser = if ($env:POSTGRES_USER)     { $env:POSTGRES_USER }     else { "postgres" }
    $dbPass = if ($env:POSTGRES_PASSWORD) { $env:POSTGRES_PASSWORD } else { "postgres" }
    $dbName = if ($env:POSTGRES_DB)       { $env:POSTGRES_DB }       else { "agent_db" }
    $DB_URL = "postgres://${dbUser}:${dbPass}@${dbHost}:${dbPort}/${dbName}"
}

# =====================================================================
# JWT / 管理员兜底
# =====================================================================
# .env 里是 JWT_SECRET，Go 读 SECRET_KEY，这里做映射
if (-not $env:SECRET_KEY) {
    $env:SECRET_KEY = if ($env:JWT_SECRET) { $env:JWT_SECRET } else { "dev-secret-key" }
}
if (-not $env:ADMIN_USERNAME) { $env:ADMIN_USERNAME = "admin" }
if (-not $env:ADMIN_PASSWORD) { $env:ADMIN_PASSWORD = "admin123" }

# =====================================================================
# 1. 编译后端
# =====================================================================
Write-Host "[1/3] 编译后端 ..." -ForegroundColor Cyan
Push-Location $BackendDir
try {
    go build -o $BackendExe ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "后端编译失败" }
}
finally { Pop-Location }

# =====================================================================
# 2. 启动后端
# =====================================================================
Write-Host "[2/3] 启动后端 http://localhost:$BackendPort ..." -ForegroundColor Cyan
$env:DATABASE_URL = $DB_URL
$env:MCP_PORT     = "$BackendPort"
$env:LOG_FORMAT   = "text"

$backend = Start-Process `
    -FilePath $BackendExe `
    -WorkingDirectory $BackendDir `
    -PassThru

Write-Host "  后端 PID=$($backend.Id)（日志在其独立控制台窗口输出）" -ForegroundColor DarkGray
Start-Sleep -Seconds 2

if ($backend.HasExited) {
    Write-Host "  后端启动失败（可手动运行 $BackendExe 查看报错）" -ForegroundColor Red
    throw "后端启动失败，请检查数据库连接配置"
}

# =====================================================================
# 3. 启动前端 dev（前台运行，Ctrl+C 停止）
# =====================================================================
Write-Host "[3/3] 启动前端 dev http://localhost:$FrontendPort ..." -ForegroundColor Cyan
Write-Host "  登录账号: $($env:ADMIN_USERNAME)" -ForegroundColor DarkGray

Push-Location $FrontendDir
try {
    if (Get-Command pnpm -ErrorAction SilentlyContinue) {
        pnpm dev
    } else {
        npm run dev
    }
}
finally {
    Pop-Location
    Write-Host "`n停止后端 ..." -ForegroundColor Yellow
    Stop-Process -Id $backend.Id -Force -ErrorAction SilentlyContinue
}
