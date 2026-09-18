[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

function Assert-NativeSuccess([string]$Step) {
    if ($LASTEXITCODE -ne 0) {
        throw "$Step falló con código $LASTEXITCODE."
    }
}
$root = Split-Path -Parent $PSScriptRoot
$bin = Join-Path $root "bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null

Push-Location $root
try {
    go build -trimpath -o (Join-Path $bin "scopeth-engine.exe") ./cmd/scopeth-engine
    Assert-NativeSuccess "go build"
    uv sync --project (Join-Path $root "python") --all-groups --locked
    Assert-NativeSuccess "uv sync"
} finally {
    Pop-Location
}

Write-Host "Build completado: $bin"
