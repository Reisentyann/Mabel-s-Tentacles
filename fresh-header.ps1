# fresh-header.ps1 —— 刷新 Go 文件头部的"修改"日期为今天。
#
# 用法：
#   ./fresh-header.ps1            # 刷新全部 Go 文件的头部日期
#   ./fresh-header.ps1 -Changed   # 只刷新 git 工作区有改动/新增的 .go 文件（推荐）
#
# 头部格式见任一 .go 文件顶部两行（// 文件：… / // 修改：…）。
# 只改日期不动其他内容；无头部的文件跳过（加头请人工补"// 文件："行）。

param([switch]$Changed)

$root = $PSScriptRoot
$today = Get-Date -Format 'yyyy-MM-dd'

$targets = Get-ChildItem -Recurse -Filter *.go "$root\describer-go", "$root\indexer-go", "$root\manager-go", "$root\mcp-server-go"
if ($Changed) {
    $dirty = @(git -C $root status --porcelain -- '*.go' | ForEach-Object { ($_ -replace '^[ ?AMDR]+\s+', '').Replace('/', '\') })
    $targets = $targets | Where-Object { $dirty -contains $_.FullName.Substring($root.Length + 1) }
}

$utf8 = [System.Text.UTF8Encoding]::new($false)
$n = 0
foreach ($f in $targets) {
    $text = [System.IO.File]::ReadAllText($f.FullName)
    $new = [regex]::Replace($text, '(?m)^// 修改：\d{4}-\d{2}-\d{2}', "// 修改：$today")
    if ($new -ne $text) {
        [System.IO.File]::WriteAllText($f.FullName, $new, $utf8)
        $n++
    }
}
Write-Host "fresh-header: 已刷新 $n 个文件的修改日期 → $today（共 $($targets.Count) 个候选）"
