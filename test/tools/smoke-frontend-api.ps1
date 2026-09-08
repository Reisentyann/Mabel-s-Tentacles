# smoke-frontend-api.ps1 — 前端消费面 API 冒烟：树/详情/move（单页管理台的关键链路）
$ErrorActionPreference = "Stop"
$Root = $PSScriptRoot | Split-Path | Split-Path

Get-Content (Join-Path $Root ".env") | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim(), "Process")
    }
}
$env:DATA_DIR     = Join-Path $Root "data"
$env:LOG_FILE     = Join-Path $Root "logs\server.log"
$env:DATABASE_URL = "postgres://$($env:POSTGRES_USER):$($env:POSTGRES_PASSWORD)@localhost:54322/$($env:POSTGRES_DB)"
$env:MCP_PORT     = "8080"
$env:MCP_BASE_URL = "http://127.0.0.1:8080"
$env:LOG_FORMAT   = "text"
$env:SECRET_KEY   = if ($env:JWT_SECRET) { $env:JWT_SECRET } else { "dev-secret-key" }

$Exe = Join-Path $Root "mcp-server-go\bin\server.exe"
# 冒烟必须用当前源码（无条件重编：move 等新端点不在旧 exe 里）
Push-Location (Join-Path $Root "mcp-server-go"); go build -o $Exe ./cmd/server; Pop-Location

$server = Start-Process -FilePath $Exe -WorkingDirectory (Join-Path $Root "mcp-server-go") `
    -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput (Join-Path $Root "test\smoke-out.log") `
    -RedirectStandardError (Join-Path $Root "test\smoke-err.log")
try {
    Start-Sleep 3
    $loginBody = [Text.Encoding]::UTF8.GetBytes(('{"username":"' + $env:ADMIN_USERNAME + '","password":"' + $env:ADMIN_PASSWORD + '"}'))
    $login = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/auth/login" `
        -ContentType "application/json; charset=utf-8" -Body $loginBody
    $H = @{ Authorization = "Bearer $($login.access_token)" }

    # ① 树（前端 el-tree 的数据源形状：{tree:[{name,path,type,size_bytes,children}]}）
    $tree = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/files" -Headers $H
    Write-Host "TREE 根节点数: $($tree.tree.Count) 首节点: $($tree.tree[0].name) type=$($tree.tree[0].type)"

    $script:leaf = @()
    function Walk($nodes) {
        foreach ($n in $nodes) {
            if ($n.type -eq "dir") { Walk $n.children } else { $script:leaf += $n }
        }
    }
    Walk $tree.tree
    Write-Host "叶子文件数: $($leaf.Count) 示例: $($leaf[0].path) size_bytes=$($leaf[0].size_bytes)"

    # ② 详情（抽屉数据源：uuid/copied_from/moved_from/attributes）
    $meta = Invoke-RestMethod -Uri ("http://127.0.0.1:8080/api/files/metadata?path=" + [uri]::EscapeDataString($leaf[0].path)) -Headers $H
    Write-Host "META: uuid=$($meta.uuid.Substring(0,8))… copied_from=$($meta.copied_from) moved_from=$($meta.moved_from) attrs键数=$($meta.attributes.PSObject.Properties.Count)"

    # ③ move（ext 变化 → 物理位搬移；再移回 → 归位）
    $newPath = $leaf[0].path -replace "\.[^.]+$", "-moved-smoke.md"
    $mvBody = [Text.Encoding]::UTF8.GetBytes(('{"from":"' + $leaf[0].path + '","to":"' + $newPath + '"}'))
    $mv = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/files/move" `
        -ContentType "application/json; charset=utf-8" -Headers $H -Body $mvBody
    Write-Host "MOVE: success=$($mv.success) storage_move=$($mv.storage_move) uuid=$($mv.uuid.Substring(0,8))…"

    $backBody = [Text.Encoding]::UTF8.GetBytes(('{"from":"' + $newPath + '","to":"' + $leaf[0].path + '"}'))
    $mv2 = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:8080/api/files/move" `
        -ContentType "application/json; charset=utf-8" -Headers $H -Body $backBody
    Write-Host "MOVE-BACK: success=$($mv2.success) storage_move=$($mv2.storage_move)（物理位归位）"
    Write-Host "SMOKE ALL PASS" -ForegroundColor Green
} finally {
    Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
}
