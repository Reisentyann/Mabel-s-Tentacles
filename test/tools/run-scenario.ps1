# run-scenario.ps1 — L4 全流程场景测试（构思自驱）：起真服务（master key 鉴权模式）→ scenario 客户端双通道跑书房档案场景
# 用法：仓库根执行  pwsh -NoProfile -File .\test\tools\run-scenario.ps1
# 前置：docker start agent_postgres（127.0.0.1:54322）

$ErrorActionPreference = "Stop"
$Root       = $PSScriptRoot | Split-Path | Split-Path
$BackendDir = Join-Path $Root "mcp-server-go"
$ClientDir  = Join-Path $Root "test\tools\scenario"
$DataDir    = Join-Path $Root "data"
$Exe        = Join-Path $BackendDir "bin\server.exe"
$LogOut     = Join-Path $Root "test\scenario-server-out.log"

# 与 run-e2e 同源的 env 装配
Get-Content (Join-Path $Root ".env") | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim(), "Process")
    }
}
$env:DATA_DIR   = $DataDir
$env:LOG_FILE   = Join-Path $Root "logs\server.log"
$env:DATABASE_URL = "postgres://$($env:POSTGRES_USER):$($env:POSTGRES_PASSWORD)@localhost:54322/$($env:POSTGRES_DB)"
$env:MCP_PORT     = "8080"
$env:MCP_BASE_URL = "http://127.0.0.1:8080"
$env:LOG_FORMAT   = "text"
$env:SECRET_KEY    = if ($env:JWT_SECRET) { $env:JWT_SECRET } else { "dev-secret-key" }
# 场景与 e2e 的差异：强制 master key 鉴权模式（顺带测 MCP 通道真鉴权路径，
# 而非开发放行）。external key 由客户端经 admin 端点现签。
$env:MCP_API_KEY  = "scenario-master-key-20260908"

Write-Host "[1/3] 编译后端与场景客户端 ..." -ForegroundColor Cyan
Push-Location $BackendDir; go build -o $Exe ./cmd/server; Pop-Location
Push-Location $ClientDir;  go build .;               Pop-Location

Write-Host "[2/3] 启动后端 :8080（master key 鉴权模式）..." -ForegroundColor Cyan
$server = Start-Process -FilePath $Exe -WorkingDirectory $BackendDir `
    -RedirectStandardOutput $LogOut -RedirectStandardError (Join-Path $Root "test\scenario-server-err.log") `
    -PassThru -WindowStyle Hidden
try {
    $healthy = $false
    foreach ($i in 1..20) {
        Start-Sleep -Milliseconds 500
        try {
            Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 2 | Out-Null
            $healthy = $true; break
        } catch { }
    }
    if (-not $healthy) { throw "后端健康检查未通过，日志见 $LogOut" }

    Write-Host "[3/3] 场景客户端：书房档案全流程 ..." -ForegroundColor Cyan
    & (Join-Path $ClientDir "scenario.exe") `
        -url "http://127.0.0.1:8080/sse" -master $env:MCP_API_KEY `
        -api "http://127.0.0.1:8080" `
        -admin-user $env:ADMIN_USERNAME -admin-pass $env:ADMIN_PASSWORD
    if ($LASTEXITCODE -ne 0) { throw "场景测试有断言失败（见上方 FAIL 行）" }
    Write-Host "场景全流程通过：书房档案 + 移动两式 + 修改重析 + 多用户同名碰撞 + 跨用户寻址 + 谱系" -ForegroundColor Green
} finally {
    if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
    Write-Host "后端已停止（日志：$LogOut）" -ForegroundColor DarkGray
}
