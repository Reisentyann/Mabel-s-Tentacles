# run-vizdump.ps1 — 开发者观察工具装配：读 .env → 拼 DATABASE_URL → 编译并跑 vizdump → 打开报告
# 用法：仓库根执行  pwsh -NoProfile -File .\test\tools\run-vizdump.ps1
# 前置：docker start agent_postgres（127.0.0.1:54322）
# 产物：仓库根 fileviz.html（浏览器自包含报告）+ fileviz.json（机器可读原始导出）

$ErrorActionPreference = "Stop"
$Root       = $PSScriptRoot | Split-Path | Split-Path   # test/tools → 项目根
$BackendDir = Join-Path $Root "mcp-server-go"

Get-Content (Join-Path $Root ".env") | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim(), "Process")
    }
}
$env:DATA_DIR     = Join-Path $Root "data"
$env:DATABASE_URL = "postgres://$($env:POSTGRES_USER):$($env:POSTGRES_PASSWORD)@localhost:54322/$($env:POSTGRES_DB)"

Write-Host "[1/2] 编译 vizdump ..." -ForegroundColor Cyan
Push-Location $BackendDir; go build -o bin\vizdump.exe ./cmd/vizdump; Pop-Location

Write-Host "[2/2] 导出报告（DB 全量 + 盘面核对 + sha-256 对账）..." -ForegroundColor Cyan
& (Join-Path $BackendDir "bin\vizdump.exe") `
    -env (Join-Path $Root ".env") -data $env:DATA_DIR `
    -o (Join-Path $Root "fileviz.html") -json (Join-Path $Root "fileviz.json")
if ($LASTEXITCODE -ne 0) { throw "vizdump 导出失败（见上方错误）" }

Invoke-Item (Join-Path $Root "fileviz.html")
Write-Host "报告已在浏览器打开：$(Join-Path $Root 'fileviz.html')（JSON：fileviz.json）" -ForegroundColor Green
