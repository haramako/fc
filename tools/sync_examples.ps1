# examples/ と実プロジェクト (C:\Work\fc-miku, C:\Work\castle) の差分確認・同期スクリプト
#
# 使い方:
#   .\tools\sync_examples.ps1              # 差分表示のみ (git diff --no-index)
#   .\tools\sync_examples.ps1 -Update      # 実プロジェクト → examples/ へコピー (取り込み更新)
#   .\tools\sync_examples.ps1 -MikuDir D:\path\fc-miku -CastleDir D:\path\castle
#
# 対象ファイルは「examples/ に存在するファイル」(= 取り込み済みの一覧が正)。
# 実プロジェクト側で追加されたファイルは自動では取り込まないので、
# 必要になったらファイルを examples/ に置いてから -Update で追従させる。
#
# examples/ 側だけを意図的に変更した場合は、このスクリプトの差分表示で
# その乖離を確認できる (詳細は examples/README.md)。

param(
    [switch]$Update,
    [string]$MikuDir = "C:\Work\fc-miku",
    [string]$CastleDir = "C:\Work\castle"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

# examples/<name> → 実プロジェクトのルート
$projects = @{
    "miku"   = $MikuDir
    "castle" = $CastleDir
}

$diffCount = 0
$missingCount = 0

foreach ($name in $projects.Keys | Sort-Object) {
    $exampleRoot = Join-Path $root "examples\$name"
    $realRoot = $projects[$name]
    if (-not (Test-Path $realRoot)) {
        Write-Host "SKIP: $name (実プロジェクトが見つからない: $realRoot)" -ForegroundColor Yellow
        continue
    }
    Write-Host "=== $name ( $realRoot ) ===" -ForegroundColor Cyan

    Get-ChildItem -Recurse -File $exampleRoot | Where-Object {
        # ビルド生成物は対象外
        $_.FullName -notmatch '\\\.fc-build\\'
    } | ForEach-Object {
        $rel = $_.FullName.Substring($exampleRoot.Length + 1)
        $real = Join-Path $realRoot $rel
        if (-not (Test-Path $real)) {
            Write-Host "MISSING in real project: $name\$rel" -ForegroundColor Yellow
            $script:missingCount++
            return
        }
        if ($Update) {
            Copy-Item $real $_.FullName -Force
        } else {
            # git diff --no-index はエンコーディング非依存で差分表示できる
            git --no-pager diff --no-index --stat -- $real $_.FullName
            if ($LASTEXITCODE -ne 0) { $script:diffCount++ }
        }
    }
}

if ($Update) {
    Write-Host ""
    Write-Host "実プロジェクトの内容を examples/ に反映した。差分は git status / git diff で確認すること。" -ForegroundColor Green
} else {
    Write-Host ""
    if ($diffCount -eq 0 -and $missingCount -eq 0) {
        Write-Host "差分なし: examples/ は実プロジェクトと一致している" -ForegroundColor Green
    } else {
        Write-Host "$diffCount 件の差分 / $missingCount 件の欠落。詳細は 'git diff --no-index <real> <example>' で確認。" -ForegroundColor Yellow
    }
}
