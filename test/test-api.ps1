# test/test-api.ps1 — 后端主要接口功能测试
# 不只测"通不通"，还校验数据正确性：
#   - 文件树与磁盘实际文件一致
#   - 下载内容与磁盘字节一致
#   - zip 包内确实含指定文件
#   - JWT 可解析、sub 正确
#   - refresh 后旧 token 被吊销（复用 -> 401）
# 用法：pwsh -File test/test-api.ps1
# 结果：控制台 + test/test-result.log

param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$ResultFile = ""
)

$ErrorActionPreference = "Stop"

if (-not $ResultFile) { $ResultFile = Join-Path $PSScriptRoot "test-result.log" }

# 定位项目根 + data 目录（与 start-dev.ps1 相同的解析逻辑）
$Root = $PSScriptRoot | Split-Path -Parent
$dataDir = Join-Path $Root "data"

# 读 .env 管理员账号
$envFile = Join-Path $Root ".env"
$adminUser = "admin"
$adminPass = "admin123"
if (Test-Path -LiteralPath $envFile) {
    Get-Content -LiteralPath $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line -match "^ADMIN_USERNAME=(.*)$") { $adminUser = $Matches[1] }
        if ($line -and -not $line.StartsWith("#") -and $line -match "^ADMIN_PASSWORD=(.*)$") { $adminPass = $Matches[1] }
    }
}

$pass = 0
$fail = 0

function Check {
    param([string]$Name, [bool]$Ok, [string]$Detail = "")
    if ($Ok) {
        $script:pass++
        $line = "[PASS] $Name"
        Write-Host "  $line" -ForegroundColor Green
    } else {
        $script:fail++
        $line = "[FAIL] $Name  $Detail"
        Write-Host "  $line" -ForegroundColor Red
    }
    Add-Content -LiteralPath $ResultFile -Value $line -Encoding UTF8
}

function Get-Url([string]$Path) { return "$BaseUrl$Path" }

function Flatten-Tree {
    param($Nodes)
    foreach ($n in $Nodes) {
        $n
        if ($n.children) { Flatten-Tree $n.children }
    }
}

function Test-BytesEqual {
    param([byte[]]$A, [byte[]]$B)
    if ($A.Length -ne $B.Length) { return $false }
    for ($i = 0; $i -lt $A.Length; $i++) { if ($A[$i] -ne $B[$i]) { return $false } }
    return $true
}

Set-Content -LiteralPath $ResultFile -Value "== 后端接口功能测试 $BaseUrl  $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') ==" -Encoding UTF8
Add-Content -LiteralPath $ResultFile -Value "data 目录: $dataDir" -Encoding UTF8
Add-Content -LiteralPath $ResultFile -Value "" -Encoding UTF8

Write-Host ""
Write-Host "== 后端接口功能测试  $BaseUrl ==" -ForegroundColor Cyan
Write-Host "data 目录: $dataDir" -ForegroundColor DarkGray
Write-Host ""

# ---------- 1. health ----------
try {
    $r = Invoke-WebRequest -Uri (Get-Url "/health") -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    Check "GET /health" ($r.StatusCode -eq 200)
} catch { Check "GET /health" $false $_.Exception.Message }

# ---------- 2. 文件树 = 磁盘实际文件 ----------
try {
    $r = Invoke-WebRequest -Uri (Get-Url "/api/files/") -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    $json = $r.Content | ConvertFrom-Json
    $apiFiles = @(Flatten-Tree $json.tree | Where-Object { $_.type -eq "file" } | ForEach-Object { $_.path })
    $diskFiles = @(Get-ChildItem -LiteralPath $dataDir -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
        $_.FullName.Substring($dataDir.Length + 1).Replace("\", "/")
    })
    $missing = @($diskFiles | Where-Object { $_ -notin $apiFiles })
    $extra = @($apiFiles | Where-Object { $_ -notin $diskFiles })
    $ok = $r.StatusCode -eq 200 -and $missing.Count -eq 0 -and $extra.Count -eq 0
    Check "GET /api/files/ (tree=磁盘文件)" $ok "API=$($apiFiles -join ',') 缺失=$($missing -join ',') 多余=$($extra -join ',')"
} catch { Check "GET /api/files/ (tree=磁盘文件)" $false $_.Exception.Message }

# ---------- 3. 下载内容 = 磁盘字节 ----------
try {
    $tmpDown = Join-Path $env:TEMP "test-dl-jokes.txt"
    Invoke-WebRequest -Uri (Get-Url "/api/files/download?path=jokes.txt") -OutFile $tmpDown -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    $apiBytes = [System.IO.File]::ReadAllBytes($tmpDown)
    $diskBytes = [System.IO.File]::ReadAllBytes((Join-Path $dataDir "jokes.txt"))
    $same = Test-BytesEqual $apiBytes $diskBytes
    Check "GET /api/files/download (内容字节一致)" $same "API=$($apiBytes.Length)B 磁盘=$($diskBytes.Length)B"
} catch { Check "GET /api/files/download (内容字节一致)" $false $_.Exception.Message }

# ---------- 4. 下载不存在 -> 404 ----------
try {
    $r = Invoke-WebRequest -Uri (Get-Url "/api/files/download?path=no_such.txt") -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    Check "GET /api/files/download (不存在->404)" ($r.StatusCode -eq 404) "status=$($r.StatusCode)"
} catch { Check "GET /api/files/download (404)" $false $_.Exception.Message }

# ---------- 5. zip 包内含指定文件且内容一致 ----------
try {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $tmpZip = Join-Path $env:TEMP "test-dl.zip"
    Invoke-WebRequest -Uri (Get-Url "/api/files/download-zip") -Method POST `
        -Body '{"paths":["jokes.txt","mabel_intro2.html"]}' -ContentType "application/json" `
        -OutFile $tmpZip -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    $zip = [System.IO.Compression.ZipFile]::OpenRead($tmpZip)
    $names = @($zip.Entries | ForEach-Object { $_.FullName })
    $zip.Dispose()
    $need = @("jokes.txt", "mabel_intro2.html")
    $missingZip = @($need | Where-Object { $_ -notin $names })
    Check "POST /api/files/download-zip (含指定文件)" ($missingZip.Count -eq 0) "zip内=$($names -join ',')"
} catch { Check "POST /api/files/download-zip" $false $_.Exception.Message }

# ---------- 6. 操作存档：结构 + 分页 + tool_name 非空 ----------
try {
    $r = Invoke-WebRequest -Uri (Get-Url "/api/operations/?page=1&size=20") -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    $json = $r.Content | ConvertFrom-Json
    $hasPaging = $null -ne $json.items -and $null -ne $json.total -and $null -ne $json.page -and $null -ne $json.size
    $badTool = @($json.items | Where-Object { -not $_.tool_name }).Count
    $ok = $r.StatusCode -eq 200 -and $hasPaging -and $badTool -eq 0
    Check "GET /api/operations/ (结构+分页+tool_name)" $ok "total=$($json.total) 缺tool=$badTool"
} catch { Check "GET /api/operations/" $false $_.Exception.Message }

# ---------- 7. 命令列表 ----------
$firstCommand = $null
try {
    $r = Invoke-WebRequest -Uri (Get-Url "/api/commands/?page=1&size=10") -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    $json = $r.Content | ConvertFrom-Json
    $ok = $r.StatusCode -eq 200 -and $null -ne $json.items -and $null -ne $json.total
    if ($ok -and $json.items.Count -gt 0) { $firstCommand = $json.items[0] }
    $bad = @($json.items | Where-Object { -not $_.command_text }).Count
    $ok = $ok -and $bad -eq 0
    Check "GET /api/commands/ (结构+command_text)" $ok "total=$($json.total) 缺text=$bad"
} catch { Check "GET /api/commands/" $false $_.Exception.Message }

# ---------- 8. 命令详情 = 列表项 ----------
if ($firstCommand) {
    try {
        $r = Invoke-WebRequest -Uri (Get-Url "/api/commands/$($firstCommand.id)") -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
        $json = $r.Content | ConvertFrom-Json
        $ok = $r.StatusCode -eq 200 -and $json.id -eq $firstCommand.id -and $json.command_text -eq $firstCommand.command_text
        Check "GET /api/commands/{id} (与列表一致)" $ok "id=$($json.id)"
    } catch { Check "GET /api/commands/{id}" $false $_.Exception.Message }
} else {
    Check "GET /api/commands/{id} (与列表一致)" $false "命令列表为空，跳过"
}

# ---------- 9. 登录：JWT 可解析、sub 正确 ----------
$accessToken = $null
$refreshToken = $null
try {
    $body = @{ username = $adminUser; password = $adminPass } | ConvertTo-Json
    $r = Invoke-WebRequest -Uri (Get-Url "/api/auth/login") -Method POST -Body $body `
        -ContentType "application/json" -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    $json = $r.Content | ConvertFrom-Json
    $accessToken = $json.access_token
    $refreshToken = $json.refresh_token
    $jwtOk = $false
    if ($accessToken) {
        $parts = $accessToken.Split(".")
        if ($parts.Count -eq 3) {
            $pad = $parts[1].Replace("-", "+").Replace("_", "/")
            while ($pad.Length % 4 -ne 0) { $pad += "=" }
            $payload = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($pad)) | ConvertFrom-Json
            $jwtOk = $payload.sub -eq $adminUser -and $null -ne $payload.exp
        }
    }
    Check "POST /api/auth/login (JWT 可解析+sub正确)" ($r.StatusCode -eq 200 -and $jwtOk) "status=$($r.StatusCode) sub=$($payload.sub)"
} catch { Check "POST /api/auth/login (JWT)" $false $_.Exception.Message }

# ---------- 10. refresh：换新 token + 旧 token 被吊销 ----------
if ($refreshToken) {
    try {
        $body = @{ refresh_token = $refreshToken } | ConvertTo-Json
        $r1 = Invoke-WebRequest -Uri (Get-Url "/api/auth/refresh") -Method POST -Body $body `
            -ContentType "application/json" -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
        $j1 = $r1.Content | ConvertFrom-Json
        $firstOk = $r1.StatusCode -eq 200 -and $j1.access_token
        $r2 = Invoke-WebRequest -Uri (Get-Url "/api/auth/refresh") -Method POST -Body $body `
            -ContentType "application/json" -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
        $revokedOk = $r2.StatusCode -eq 401
        Check "POST /api/auth/refresh (换新+旧token吊销->401)" ($firstOk -and $revokedOk) "首次=$($r1.StatusCode) 复用旧=$($r2.StatusCode)"
    } catch { Check "POST /api/auth/refresh" $false $_.Exception.Message }
} else {
    Check "POST /api/auth/refresh (换新+吊销)" $false "登录未成功"
}

# ---------- 11. register -> 403 ----------
try {
    $r = Invoke-WebRequest -Uri (Get-Url "/api/auth/register") -Method POST -Body '{}' `
        -ContentType "application/json" -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
    Check "POST /api/auth/register (禁用->403)" ($r.StatusCode -eq 403) "status=$($r.StatusCode)"
} catch { Check "POST /api/auth/register" $false $_.Exception.Message }

Write-Host ""
Write-Host "== 结果：$pass 通过 / $fail 失败 ==" -ForegroundColor Cyan
Add-Content -LiteralPath $ResultFile -Value "" -Encoding UTF8
Add-Content -LiteralPath $ResultFile -Value "== 结果：$pass 通过 / $fail 失败 ==" -Encoding UTF8
if ($fail -gt 0) { exit 1 }