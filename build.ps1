# 构建 PingHistory（前端产物 → go:embed → 单文件可执行程序）
#
# 用法：
#   pwsh -File .\build.ps1                       # 默认构建 Windows x64 + Linux ARMv8
#   pwsh -File .\build.ps1 -Target windows       # 只构建 Windows x64
#   pwsh -File .\build.ps1 -Target linux-arm64   # 只构建 Linux ARMv8（OpenWrt 路由器）
#   pwsh -File .\build.ps1 -SkipWeb              # 跳过前端构建（只改了 Go 代码时）

param(
    [ValidateSet('all', 'windows', 'linux-arm64')]
    [string]$Target = 'all',
    [switch]$SkipWeb
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path

function Get-BuildSpec([string]$name) {
    switch ($name) {
        'windows' { return @{ OS = 'windows'; Arch = 'amd64'; Out = 'dist\PingHistory.exe'; Ld = '-H=windowsgui -s -w' } }
        'linux-arm64' { return @{ OS = 'linux'; Arch = 'arm64'; Out = 'dist\PingHistory-linux-arm64'; Ld = '-s -w' } }
    }
}

$specs = if ($Target -eq 'all') { @('windows', 'linux-arm64') } else { @($Target) }

if (-not $SkipWeb) {
    Write-Host '==> [1/2] 构建前端 (web)' -ForegroundColor Cyan
    Push-Location (Join-Path $root 'web')
    try {
        if (-not (Test-Path 'node_modules')) {
            npm install --no-fund --no-audit
        }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw '前端构建失败' }
    }
    finally {
        Pop-Location
    }
}
else {
    Write-Host '==> 跳过前端构建（使用现有 web\dist）' -ForegroundColor DarkGray
}

Write-Host '==> [2/2] 编译可执行文件' -ForegroundColor Cyan
Push-Location $root
try {
    New-Item -ItemType Directory -Force -Path (Join-Path $root 'dist') | Out-Null
    $results = @()

    foreach ($name in $specs) {
        $spec = Get-BuildSpec $name
        Write-Host ("  · {0}/{1}" -f $spec.OS, $spec.Arch) -ForegroundColor DarkGray

        $env:CGO_ENABLED = '0'
        $env:GOOS = $spec.OS
        $env:GOARCH = $spec.Arch
        if ($spec.Arch -eq 'arm') { $env:GOARM = '7' } else { Remove-Item Env:GOARM -ErrorAction SilentlyContinue }
        $out = Join-Path $root $spec.Out
        go build -trimpath -ldflags $spec.Ld -o $out .
        if ($LASTEXITCODE -ne 0) { throw ("构建失败：{0}/{1}" -f $spec.OS, $spec.Arch) }

        $size = [math]::Round((Get-Item $out).Length / 1MB, 2)
        $results += [pscustomobject]@{ 平台 = "{0}/{1}" -f $spec.OS, $spec.Arch; 文件 = $spec.Out; 体积 = "$size MB" }
    }
}
finally {
    Pop-Location
    Remove-Item Env:GOOS, Env:GOARCH, Env:GOARM -ErrorAction SilentlyContinue
}

Write-Host ''
$results | Format-Table -AutoSize | Out-String | Write-Host
Write-Host '构建完成。' -ForegroundColor Green
Write-Host '产物均在 dist\ 下：Windows 双击 dist\PingHistory.exe；Linux 部署见 README「部署到 OpenWrt」。' -ForegroundColor DarkGray
