# run-idx-e2e.ps1 — L4 索引链路端到端（测试规则.md 第 6 节）：起真服务 → idxclient 造料+目录断言+条件断言+权限链路断言 → 重启服务 → -verify 验证 RebuildIndex 从 DB 重建后查询仍命中
# 用法：仓库根执行  pwsh -NoProfile -File .\test\tools\run-idx-e2e.ps1
# 前置：docker compose up -d postgres（127.0.0.1:54322）

$ErrorActionPreference = "Stop"
$Root       = $PSScriptRoot | Split-Path | Split-Path   # test/tools → 项目根
$BackendDir = Join-Path $Root "mcp-server-go"
$ClientDir  = Join-Path $Root "test\tools\idxclient"
$DataDir    = Join-Path $Root "data"
$Exe        = Join-Path $BackendDir "bin\server.exe"
$LogOut     = Join-Path $Root "test\idx-e2e-server-out.log"
$LogErr     = Join-Path $Root "test\idx-e2e-server-err.log"

# =====================================================================
# 读 .env（与 start-dev.ps1 / run-e2e.ps1 同源逻辑）
# =====================================================================
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

# MCP 通道鉴权：本 e2e 全程带 master key（覆盖 .env 的 MCP_API_KEY 口径）——
# 匿名开发放行关闭，idxclient 的 master/agent key 两类会话都走真鉴权路径
# （权限链路断言的复判边界才有意义；权限段注册的 idxperm 用户与
# perm批次/ 夹具跨轮幂等，重复运行无副作用）
$env:MCP_API_KEY  = "idx-e2e-master-key"
$IdxKey           = $env:MCP_API_KEY

function Start-Backend {
    $p = Start-Process -FilePath $Exe -WorkingDirectory $BackendDir `
        -RedirectStandardOutput $LogOut -RedirectStandardError $LogErr `
        -PassThru -WindowStyle Hidden
    $healthy = $false
    foreach ($i in 1..30) {
        Start-Sleep -Milliseconds 500
        try {
            Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 2 | Out-Null
            $healthy = $true; break
        } catch { }
    }
    if (-not $healthy) {
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        throw "后端健康检查未通过，日志见 $LogOut"
    }
    return $p
}

try {
    # =================================================================
    # [1/5] 编译
    # =================================================================
    Write-Host "[1/5] 编译后端与 idxclient ..." -ForegroundColor Cyan
    Push-Location $BackendDir; go build -o $Exe ./cmd/server; Pop-Location
    Push-Location $ClientDir;  go build .;               Pop-Location

    # =================================================================
    # [2/5] 第一阶段：写入 + 目录断言 + 条件断言 + 权限链路断言
    # =================================================================
    Write-Host "[2/5] 启动后端 :8080 并跑索引链路断言（写入阶段，MCP 全程真鉴权）..." -ForegroundColor Cyan
    $server = Start-Backend
    try {
        & (Join-Path $ClientDir "idxclient.exe") -url "http://127.0.0.1:8080/sse" -mkey $IdxKey -perm
        if ($LASTEXITCODE -ne 0) { throw "idxclient 写入阶段有断言失败（见上方 FAIL 行）" }
    } finally {
        if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
    }
    Write-Host "  PASS: 写入阶段全过（喂食轮询 + 目录发现 + eq/in/gt/range/And/空集/坏op + 权限复判边界 + 坏/吊销 key 拒连）" -ForegroundColor Green

    # =================================================================
    # [3/5] 重启：验证 RebuildIndex 从 DB 全量重建（索引是派生缓存）
    # =================================================================
    Write-Host "[3/5] 重启后端（启动全量重建 RebuildIndex）..." -ForegroundColor Cyan
    Start-Sleep -Seconds 1
    $server2 = Start-Backend
    try {
        # 启动日志核对：index rebuilt 行必须出现（DB 事实源 → 索引重建）
        $rebuilt = $false
        foreach ($i in 1..20) {
            if ((Test-Path $LogOut) -and (Select-String -Path $LogOut -Pattern "index rebuilt" -Quiet)) {
                $rebuilt = $true; break
            }
            Start-Sleep -Milliseconds 500
        }
        if (-not $rebuilt) { throw "启动日志未见 index rebuilt（全量重建未执行？）" }
        Write-Host "  PASS: 启动日志命中 index rebuilt（DB → 索引全量重建）" -ForegroundColor Green

        # =================================================================
        # [4/5] 第二阶段：-verify 只读断言（重建后的索引仍能圈对文件）
        # =================================================================
        Write-Host "[4/5] idxclient -verify：重建索引的只读断言 ..." -ForegroundColor Cyan
        & (Join-Path $ClientDir "idxclient.exe") -url "http://127.0.0.1:8080/sse" -mkey $IdxKey -verify
        if ($LASTEXITCODE -ne 0) { throw "idxclient verify 阶段有断言失败（重建后查询漂移？）" }
        Write-Host "  PASS: 重建后查询仍命中（索引=派生缓存，DB=事实源）" -ForegroundColor Green
    } finally {
        if ($server2 -and -not $server2.HasExited) { Stop-Process -Id $server2.Id -Force -ErrorAction SilentlyContinue }
    }

    # =================================================================
    # [5/5] 汇总
    # =================================================================
    Write-Host "[5/5] L4 索引链路全过：目录发现 + 条件查询 + 喂食时序 + 重启重建 + 权限复判边界" -ForegroundColor Green
    Write-Host "  测试产物目录：$DataDir\索引批次\（idxclient 运行产物，处置归人类）" -ForegroundColor DarkGray
} finally {
    Write-Host "后端已停止（日志：$LogOut）" -ForegroundColor DarkGray
}
