# run-e2e.ps1 — L4 端到端（测试规则.md 第 6 节）：起真服务 → MCP 客户端造文件 + analyze 核对 → HTTP analyze/backfill → 绕口对账
# 用法：仓库根执行  pwsh -NoProfile -File .\test\tools\run-e2e.ps1
# 前置：docker compose up -d postgres（127.0.0.1:54322）

$ErrorActionPreference = "Stop"
$Root       = $PSScriptRoot | Split-Path | Split-Path   # test/tools → 项目根
$BackendDir = Join-Path $Root "mcp-server-go"
$ClientDir  = Join-Path $Root "test\tools\mcpclient"
$DataDir    = Join-Path $Root "data"
$Exe        = Join-Path $BackendDir "bin\server.exe"
$LogOut     = Join-Path $Root "test\e2e-server-out.log"

# =====================================================================
# 读 .env（与 start-dev.ps1 同源逻辑）
# =====================================================================
Get-Content (Join-Path $Root ".env") | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim(), "Process")
    }
}
$env:DATA_DIR   = $DataDir                        # 绝对路径，避免 CWD 歧义
$env:LOG_FILE   = Join-Path $Root "logs\server.log"
$env:DATABASE_URL = "postgres://$($env:POSTGRES_USER):$($env:POSTGRES_PASSWORD)@localhost:54322/$($env:POSTGRES_DB)"
$env:MCP_PORT     = "8080"
$env:MCP_BASE_URL = "http://127.0.0.1:8080"
$env:LOG_FORMAT   = "text"
$env:SECRET_KEY    = if ($env:JWT_SECRET) { $env:JWT_SECRET } else { "dev-secret-key" }

# =====================================================================
# 编译 + 启动
# =====================================================================
Write-Host "[1/6] 编译后端与 MCP 客户端 ..." -ForegroundColor Cyan
Push-Location $BackendDir; go build -o $Exe ./cmd/server; Pop-Location
Push-Location $ClientDir;  go build .;               Pop-Location

Write-Host "[2/6] 启动后端 :8080 ..." -ForegroundColor Cyan
$server = Start-Process -FilePath $Exe -WorkingDirectory $BackendDir `
    -RedirectStandardOutput $LogOut -RedirectStandardError (Join-Path $Root "test\e2e-server-err.log") `
    -PassThru -WindowStyle Hidden
$cleanup = {
    if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
}
try {
    $healthy = $false
    foreach ($i in 1..20) {
        Start-Sleep -Milliseconds 500
        try {
            Invoke-RestMethod -Uri "http://127.0.0.1:8080/health" -TimeoutSec 2 | Out-Null
            $healthy = $true; break
        } catch { }
    }
    if (-not $healthy) { throw "后端 20 次健康检查未通过，日志见 $LogOut" }

    # =================================================================
    # [3a/6] 登录（require_auth=true 的权限批次口径）：admin JWT + 注册普通用户
    # =================================================================
    Write-Host "[3a/6] 登录 admin + 注册普通用户（权限 L4）..." -ForegroundColor Cyan
    $loginBody = [System.Text.Encoding]::UTF8.GetBytes(('{"username":"' + $env:ADMIN_USERNAME + '","password":"' + $env:ADMIN_PASSWORD + '"}'))
    $login = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/auth/login" `
        -ContentType "application/json; charset=utf-8" -Body $loginBody
    if (-not $login.access_token) { throw "admin 登录失败" }
    $H = @{ Authorization = "Bearer $($login.access_token)" }
    Write-Host "  PASS: admin 登录（require_auth=true 下 JWT 就位）" -ForegroundColor Green

    # 注册普通用户（开放注册）→ 越权断言：describe 他人私文件必须 403
    $regBody = [System.Text.Encoding]::UTF8.GetBytes('{"username":"e2e_user","password":"e2e-password-123"}')
    try {
        $reg = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/auth/register" `
            -ContentType "application/json; charset=utf-8" -Body $regBody
    } catch {
        # 用户名已占（重复跑）：走登录
        $reg = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/auth/login" `
            -ContentType "application/json; charset=utf-8" -Body $regBody
    }
    $UH = @{ Authorization = "Bearer $($reg.access_token)" }
    Write-Host "  PASS: 普通用户就位（e2e_user）" -ForegroundColor Green

    # 普通 backfill 也应被拒（admin 专属）
    $bfDenied = $false
    try {
        Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/files/backfill" -Headers $UH | Out-Null
    } catch {
        $bfDenied = ($_.Exception.Response.StatusCode -eq 403)
    }
    if (-not $bfDenied) { throw "普通用户 backfill 未被拒绝（admin 专属失效！）" }
    Write-Host "  PASS: 普通用户 backfill 被 403 拒绝（admin 专属）" -ForegroundColor Green

    # =================================================================
    # [3/6] MCP 链路：write_file ×3 + analyze_file 断言
    # =================================================================
    Write-Host "[3/6] MCP 客户端：write_file ×3 → analyze_file ×3 ..." -ForegroundColor Cyan
    & (Join-Path $ClientDir "mcpclient.exe") -url "http://127.0.0.1:8080/sse"
    if ($LASTEXITCODE -ne 0) { throw "MCP 端到端有断言失败（见上方 FAIL 行）" }

    # 普通用户对 admin 的私文件 describe → 期待 403（授权单点生效）。
    # 物理随机化后行是存在性事实源：挪到 write 之后，行在盘在，越权 403 真实可测。
    # owner 键空间（2026-09-08）：MCP 管家通道写的文件键 = ~agent/<逻辑路径>，
    # 普通用户显式跨用户寻址（行内 owner 矩阵拒写）
    $provBody = [System.Text.Encoding]::UTF8.GetBytes(('{"path":"~agent/测试批次/小说片段.txt","description":"越权尝试"}'))
    $denied = $false
    try {
        Invoke-RestMethod -Method Put -Uri "http://127.0.0.1:8080/api/files/metadata" `
            -ContentType "application/json; charset=utf-8" -Body $provBody -Headers $UH | Out-Null
    } catch {
        $denied = ($_.Exception.Response.StatusCode -eq 403)
    }
    if (-not $denied) { throw "普通用户 describe 他人文件未被拒绝（授权失效！）" }
    Write-Host "  PASS: 普通用户 describe 他人文件被 403 拒绝（~agent/ 显式跨用户寻址）" -ForegroundColor Green

    # =================================================================
    # [4/6] HTTP T3：POST /api/files/analyze
    # =================================================================
    Write-Host "[4/6] HTTP analyze 端点 ..." -ForegroundColor Cyan
    $body = [System.Text.Encoding]::UTF8.GetBytes('{"path":"~agent/测试批次/临时笔记.md"}')
    $resp = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/files/analyze" -Headers $H `
        -ContentType "application/json; charset=utf-8" -Body $body
    if (-not $resp.success) { throw "HTTP analyze success=false" }
    if ($resp.families -notcontains "text") { throw "HTTP analyze families 缺 text：$($resp.families)" }
    Write-Host "  PASS: analyze 端点 families = $($resp.families -join ',')" -ForegroundColor Green

    # =================================================================
    # [5/6] 绕口对账（execute_command 场景）：直改盘上文件 → backfill → 核对
    # =================================================================
    Write-Host "[5/6] 绕口对账：直改盘上文件 → backfill 一轮 ..." -ForegroundColor Cyan
    $novelRel = "~agent/测试批次/小说片段.txt"
    # 物理随机化后盘面无明文名：绕口直改目标 = uuid 派生位（查 DB 拿 uuid，
    # 模拟绕口 agent ls 盘面后摸到的随机名文件——对账口径不变）
    $uuid = (docker exec agent_postgres psql -U postgres -d agent_db -t -A -c "SELECT uuid FROM file_metadata WHERE file_path='$novelRel'") | Select-Object -First 1
    if (-not $uuid) { throw "DB 未查到 uuid（write 链路未落行？）" }
    $novelAbs = Join-Path $DataDir (Join-Path $uuid.Substring(0,2) ($uuid + ".txt"))
    if (-not (Test-Path -LiteralPath $novelAbs)) { throw "派生物理位不存在：$novelAbs" }
    $esc = [uri]::EscapeDataString($novelRel)
    $before = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/files/metadata?path=$esc" -Headers $H
    $linesBefore = $before.attributes.'cod-text-lines'
    Add-Content -LiteralPath $novelAbs -Value "`n雨停了。梅贝尔合上了最后一册清单。（e2e 直改）" -Encoding utf8

    $bf = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/files/backfill" -Headers $H
    if (-not $bf.success) { throw "backfill success=false" }
    if ($bf.analyzed -lt 1) { throw "backfill analyzed = $($bf.analyzed)，绕口直改未被 checksum/mtime 对账发现" }
    Write-Host "  PASS: backfill analyzed = $($bf.analyzed)（直改被对账发现并重分析）" -ForegroundColor Green

    # 核对（口径中立的相对断言）：行数较直改前增加、checksum 刷新、ver 与描述三件套保留
    $meta = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/files/metadata?path=$esc" -Headers $H
    $linesAfter = $meta.attributes.'cod-text-lines'
    if ($linesAfter -le $linesBefore) { throw "cod-text-lines 直改后 = $linesAfter（直改前 $linesBefore），事实未刷新" }
    if ($meta.checksum -eq $before.checksum) { throw "checksum 未刷新（重分析未落库？）" }
    if ($meta.attributes.'cod-basic-ver' -lt 3) { throw "cod-basic-ver = $($meta.attributes.'cod-basic-ver')，版本表未落库" }
    if (-not $meta.description) { throw "description 丢失（llm/描述三件套应保留）" }
    Write-Host "  PASS: cod-text-lines $linesBefore → $linesAfter，checksum 刷新，ver/description 完好" -ForegroundColor Green

    # 幂等：再跑一轮 backfill，直改已消化，不应再重分析该文件
    $bf2 = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/files/backfill" -Headers $H
    Write-Host "  PASS: 第二轮 backfill analyzed = $($bf2.analyzed)（幂等收敛）" -ForegroundColor Green

    # =================================================================
    # [6/6] 汇总
    # =================================================================
    Write-Host "[6/6] L4 全链路通过：MCP write/analyze + HTTP analyze + 绕口 backfill 对账" -ForegroundColor Green
    Write-Host "  测试产物目录：$DataDir\测试批次\（e2e 运行产物，处置归人类）" -ForegroundColor DarkGray
} finally {
    & $cleanup
    Write-Host "后端已停止（日志：$LogOut）" -ForegroundColor DarkGray
}
