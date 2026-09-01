# test/test-api.ps1 — 后端主要接口功能测试
# 校验数据正确性：文件树=磁盘、下载字节一致、zip 含文件、JWT 载荷、refresh 吊销旧 token
# 用法：pwsh -File test/test-api.ps1 [-BaseUrl http://localhost:8080]
# 结果：控制台 + test/test-result.log

param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$ResultFile = ""
)

$ErrorActionPreference = "Stop"

if (-not $ResultFile) { $ResultFile = Join-Path $PSScriptRoot "test-result.log" }

$Root = $PSScriptRoot | Split-Path -Parent
$envFile = Join-Path $Root ".env"

# ---------- 读 .env 里的键 ----------
function Get-EnvValue {
    param([string]$Key, [string]$Default = "")
    if (-not (Test-Path -LiteralPath $envFile)) { return $Default }
    $m = Get-Content -LiteralPath $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line -match "^$Key=(.*)$") { return $Matches[1] }
    }
    if ($null -ne $m -and $m -ne "") { return $m }
    return $Default
}

# data 目录：与 start-dev.ps1 相同的解析逻辑（相对路径按项目根解析成绝对路径）
$dataDir = Get-EnvValue "DATA_DIR" "data"
if (-not [System.IO.Path]::IsPathRooted($dataDir)) { $dataDir = Join-Path $Root $dataDir }

$adminUser = Get-EnvValue "ADMIN_USERNAME" "admin"
$adminPass = Get-EnvValue "ADMIN_PASSWORD" "admin123"

$pass = 0
$fail = 0

# ---------- 工具函数 ----------
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

function Invoke-Api {
    param(
        [string]$Path,
        [string]$Method = "GET",
        [hashtable]$Headers = @{},
        [object]$Body = $null
    )
    $p = @{
        Uri                = Get-Url $Path
        Method             = $Method
        UseBasicParsing    = $true
        SkipHttpErrorCheck = $true
        TimeoutSec         = 8
        ErrorAction        = "Stop"
    }
    if ($Headers.Count -gt 0) { $p.Headers = $Headers }
    if ($null -ne $Body) {
        $p.ContentType = "application/json"
        $p.Body = if ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json }
    }
    return Invoke-WebRequest @p
}

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

function Decode-JwtPayload {
    param([string]$Token)
    $parts = $Token.Split(".")
    if ($parts.Count -lt 2) { return $null }
    $pad = $parts[1].Replace("-", "+").Replace("_", "/")
    while ($pad.Length % 4 -ne 0) { $pad += "=" }
    return [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($pad)) | ConvertFrom-Json
}

Set-Content -LiteralPath $ResultFile -Value "== 后端接口功能测试 $BaseUrl  $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') ==" -Encoding UTF8
Add-Content -LiteralPath $ResultFile -Value "data 目录: $dataDir" -Encoding UTF8
Add-Content -LiteralPath $ResultFile -Value "" -Encoding UTF8

Write-Host ""
Write-Host "== 后端接口功能测试  $BaseUrl ==" -ForegroundColor Cyan
Write-Host "data 目录: $dataDir" -ForegroundColor DarkGray
Write-Host ""

# ---------- 0. 预检：后端是否在线 ----------
try {
    $r = Invoke-Api -Path "/health"
    if ($r.StatusCode -ne 200) {
        Write-Host "[错误] 后端未正常响应（/health=$($r.StatusCode)），请先 ./start-dev.ps1 启动后端" -ForegroundColor Red
        exit 1
    }
} catch {
    Write-Host "[错误] 连不上后端 $BaseUrl，请先 ./start-dev.ps1 启动后端" -ForegroundColor Red
    exit 1
}

# ---------- 1. health ----------
try { Check "GET /health" ((Invoke-Api -Path "/health").StatusCode -eq 200) }
catch { Check "GET /health" $false $_.Exception.Message }

# ---------- 2. 文件树 = 磁盘实际文件 ----------
$apiFiles = @()
try {
    $r = Invoke-Api -Path "/api/files/"
    $json = $r.Content | ConvertFrom-Json
    $apiFiles = @(Flatten-Tree $json.tree | Where-Object { $_.type -eq "file" } | ForEach-Object { $_.path })
    $diskFiles = @(Get-ChildItem -LiteralPath $dataDir -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
        $_.FullName.Substring($dataDir.Length + 1).Replace("\", "/")
    })
    $missing = @($diskFiles | Where-Object { $_ -notin $apiFiles })
    $extra = @($apiFiles | Where-Object { $_ -notin $diskFiles })
    $ok = $r.StatusCode -eq 200 -and $missing.Count -eq 0 -and $extra.Count -eq 0
    Check "GET /api/files/ (tree=磁盘文件)" $ok "API=[$($apiFiles -join ',')] 缺失=[$($missing -join ',')] 多余=[$($extra -join ',')]"
} catch { Check "GET /api/files/ (tree=磁盘文件)" $false $_.Exception.Message }

# ---------- 3. 下载第一个文件：内容字节一致 ----------
if ($apiFiles.Count -gt 0) {
    $dl = $apiFiles[0]
    try {
        $tmpDown = Join-Path $env:TEMP "test-dl-$([System.IO.Path]::GetFileName($dl))"
        Invoke-WebRequest -Uri (Get-Url "/api/files/download?path=$([uri]::EscapeDataString($dl))") -OutFile $tmpDown -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
        $apiBytes = [System.IO.File]::ReadAllBytes($tmpDown)
        $diskBytes = [System.IO.File]::ReadAllBytes((Join-Path $dataDir $dl))
        Check "GET /api/files/download (字节一致: $dl)" (Test-BytesEqual $apiBytes $diskBytes) "API=$($apiBytes.Length)B 磁盘=$($diskBytes.Length)B"
    } catch { Check "GET /api/files/download ($dl)" $false $_.Exception.Message }
} else {
    Check "GET /api/files/download" $false "data 目录无文件，跳过"
}

# ---------- 4. 下载不存在 -> 404 ----------
try { Check "GET /api/files/download (不存在->404)" ((Invoke-Api -Path "/api/files/download?path=no_such.txt").StatusCode -eq 404) }
catch { Check "GET /api/files/download (404)" $false $_.Exception.Message }

# ---------- 5. zip 批量打包：含前两个文件 ----------
if ($apiFiles.Count -gt 0) {
    $zipPaths = @($apiFiles | Select-Object -First 2)
    $zipBody = @{ paths = $zipPaths } | ConvertTo-Json
    try {
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $tmpZip = Join-Path $env:TEMP "test-dl.zip"
        Invoke-WebRequest -Uri (Get-Url "/api/files/download-zip") -Method POST -Body $zipBody -ContentType "application/json" -OutFile $tmpZip -UseBasicParsing -SkipHttpErrorCheck -TimeoutSec 8
        $zip = [System.IO.Compression.ZipFile]::OpenRead($tmpZip)
        $names = @($zip.Entries | ForEach-Object { $_.FullName })
        $zip.Dispose()
        $missingZip = @($zipPaths | Where-Object { $_ -notin $names })
        Check "POST /api/files/download-zip (含指定文件)" ($missingZip.Count -eq 0) "期望=[$($zipPaths -join ',')] zip内=[$($names -join ',')]"
    } catch { Check "POST /api/files/download-zip" $false $_.Exception.Message }
} else {
    Check "POST /api/files/download-zip" $false "data 目录无文件，跳过"
}

# ---------- 5.5 文件描述系统：describe / get / search / copy ----------
if ($apiFiles.Count -gt 0) {
    $metaFile = $apiFiles[0]
    $metaTag = "e2e-$([DateTimeOffset]::Now.ToUnixTimeSeconds())"

    try {
        $body = @{ path = $metaFile; description = "e2e 测试描述"; tags = @($metaTag, "e2e"); file_type = "text" } | ConvertTo-Json
        $r = Invoke-Api -Path "/api/files/metadata" -Method PUT -Body $body
        Check "PUT /api/files/metadata (描述)" ($r.StatusCode -eq 200) "status=$($r.StatusCode)"
    } catch { Check "PUT /api/files/metadata (描述)" $false $_.Exception.Message }

    try {
        $r = Invoke-Api -Path "/api/files/metadata?path=$([uri]::EscapeDataString($metaFile))"
        $json = $r.Content | ConvertFrom-Json
        $ok = $r.StatusCode -eq 200 -and $json.description -eq "e2e 测试描述" -and $metaTag -in $json.tags
        Check "GET /api/files/metadata (读描述)" $ok "desc=$($json.description)"
    } catch { Check "GET /api/files/metadata (读描述)" $false $_.Exception.Message }

    try {
        $r = Invoke-Api -Path "/api/files/search?tag=$metaTag"
        $json = $r.Content | ConvertFrom-Json
        $found = @($json.items | Where-Object { $_.file_path -eq $metaFile }).Count -gt 0
        Check "GET /api/files/search (按标签)" ($r.StatusCode -eq 200 -and $found) "total=$($json.total)"
    } catch { Check "GET /api/files/search (按标签)" $false $_.Exception.Message }

    $copyTarget = "e2e-copy-$([DateTimeOffset]::Now.ToUnixTimeSeconds()).txt"
    try {
        $body = @{ source = $metaFile; target = $copyTarget } | ConvertTo-Json
        $r = Invoke-Api -Path "/api/files/copy" -Method POST -Body $body
        Check "POST /api/files/copy (复制)" ($r.StatusCode -eq 200) "status=$($r.StatusCode)"
    } catch { Check "POST /api/files/copy (复制)" $false $_.Exception.Message }

    try {
        $r = Invoke-Api -Path "/api/files/metadata?path=$([uri]::EscapeDataString($copyTarget))"
        $json = $r.Content | ConvertFrom-Json
        $ok = $r.StatusCode -eq 200 -and $json.copied_from -eq $metaFile -and $json.description -eq "e2e 测试描述"
        Check "GET /api/files/metadata (副本继承+copied_from)" $ok "copied_from=$($json.copied_from)"
    } catch { Check "GET /api/files/metadata (副本)" $false $_.Exception.Message }
} else {
    Check "文件描述系统" $false "data 目录无文件，跳过"
}

# ---------- 6. 操作存档：结构 + 分页 + tool_name 非空 ----------
try {
    $r = Invoke-Api -Path "/api/operations/?page=1&size=20"
    $json = $r.Content | ConvertFrom-Json
    $hasPaging = $null -ne $json.items -and $json.items -is [array] -and $null -ne $json.total -and $null -ne $json.page -and $null -ne $json.size
    $badTool = @($json.items | Where-Object { -not $_.tool_name }).Count
    Check "GET /api/operations/ (结构+分页+tool_name)" ($r.StatusCode -eq 200 -and $hasPaging -and $badTool -eq 0) "total=$($json.total) 缺tool=$badTool"
} catch { Check "GET /api/operations/" $false $_.Exception.Message }

# ---------- 7. 命令列表 ----------
$firstCommand = $null
try {
    $r = Invoke-Api -Path "/api/commands/?page=1&size=10"
    $json = $r.Content | ConvertFrom-Json
    $ok = $r.StatusCode -eq 200 -and $null -ne $json.items -and $json.items -is [array] -and $null -ne $json.total
    if ($ok -and $json.items.Count -gt 0) { $firstCommand = $json.items[0] }
    $bad = @($json.items | Where-Object { -not $_.command_text }).Count
    Check "GET /api/commands/ (结构+command_text)" ($ok -and $bad -eq 0) "total=$($json.total) 缺text=$bad"
} catch { Check "GET /api/commands/" $false $_.Exception.Message }

# ---------- 8. 命令详情 = 列表项 ----------
if ($firstCommand) {
    try {
        $r = Invoke-Api -Path "/api/commands/$($firstCommand.id)"
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
    $r = Invoke-Api -Path "/api/auth/login" -Method POST -Body @{ username = $adminUser; password = $adminPass }
    $json = $r.Content | ConvertFrom-Json
    $accessToken = $json.access_token
    $refreshToken = $json.refresh_token
    $payload = if ($accessToken) { Decode-JwtPayload $accessToken } else { $null }
    $jwtOk = $null -ne $payload -and $payload.sub -eq $adminUser -and $null -ne $payload.exp
    Check "POST /api/auth/login (JWT 可解析+sub正确)" ($r.StatusCode -eq 200 -and $jwtOk) "status=$($r.StatusCode) sub=$($payload.sub)"
} catch { Check "POST /api/auth/login (JWT)" $false $_.Exception.Message }

# ---------- 10. refresh：换新 token + 旧 token 被吊销 ----------
if ($refreshToken) {
    try {
        $body = @{ refresh_token = $refreshToken }
        $r1 = Invoke-Api -Path "/api/auth/refresh" -Method POST -Body $body
        $j1 = $r1.Content | ConvertFrom-Json
        $firstOk = $r1.StatusCode -eq 200 -and $j1.access_token
        $r2 = Invoke-Api -Path "/api/auth/refresh" -Method POST -Body $body
        Check "POST /api/auth/refresh (换新+旧token吊销->401)" ($firstOk -and $r2.StatusCode -eq 401) "首次=$($r1.StatusCode) 复用旧=$($r2.StatusCode)"
    } catch { Check "POST /api/auth/refresh" $false $_.Exception.Message }
} else {
    Check "POST /api/auth/refresh (换新+吊销)" $false "登录未成功"
}

# ---------- 11. 带 token 访问受保护接口 ----------
if ($accessToken) {
    try {
        $r = Invoke-Api -Path "/api/operations/" -Headers @{ Authorization = "Bearer $accessToken" }
        Check "GET /api/operations/ (带 Bearer token)" ($r.StatusCode -eq 200) "status=$($r.StatusCode)"
    } catch { Check "GET /api/operations/ (带 token)" $false $_.Exception.Message }
} else {
    Check "GET /api/operations/ (带 token)" $false "无 token"
}

# ---------- 12. register 禁用 -> 403 ----------
try { Check "POST /api/auth/register (禁用->403)" ((Invoke-Api -Path "/api/auth/register" -Method POST -Body @{}).StatusCode -eq 403) }
catch { Check "POST /api/auth/register (403)" $false $_.Exception.Message }

Write-Host ""
Write-Host "== 结果：$pass 通过 / $fail 失败 ==" -ForegroundColor Cyan
Add-Content -LiteralPath $ResultFile -Value "" -Encoding UTF8
Add-Content -LiteralPath $ResultFile -Value "== 结果：$pass 通过 / $fail 失败 ==" -Encoding UTF8
if ($fail -gt 0) { exit 1 }