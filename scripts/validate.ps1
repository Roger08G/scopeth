[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Solc
)

$ErrorActionPreference = "Stop"

function Assert-NativeSuccess([string]$Step) {
    if ($LASTEXITCODE -ne 0) {
        throw "$Step falló con código $LASTEXITCODE."
    }
}
$root = Split-Path -Parent $PSScriptRoot
$engine = Join-Path $root "bin\scopeth-engine.exe"
$fixture = Join-Path $root "fixtures\solidity"
$fixtureF4 = Join-Path $root "fixtures\f4"
$output = Join-Path ([System.IO.Path]::GetTempPath()) ("scopeth-validation-" + [guid]::NewGuid())

if (-not (Test-Path -LiteralPath $Solc -PathType Leaf)) {
    throw "solc no existe: $Solc"
}

Push-Location $root
try {
    & "$PSScriptRoot\build.ps1"
    Assert-NativeSuccess "build.ps1"
    go test ./...
    Assert-NativeSuccess "go test"
    go vet ./...
    Assert-NativeSuccess "go vet"
    uv run --project python ruff check python
    Assert-NativeSuccess "Ruff"
    uv run --project python ruff format --check python
    Assert-NativeSuccess "Ruff format"
    uv run --project python mypy --config-file python/pyproject.toml python/src python/tests
    Assert-NativeSuccess "mypy"
    uv run --project python pytest python/tests
    Assert-NativeSuccess "pytest"

    function Invoke-ScopethAnalysis([string]$FixturePath, [string]$RequestId) {
        $request = @{
            protocol_version = "1.0"
            request_id = $RequestId
            operation = "analyze"
            input = $FixturePath
            options = @{
                mode = "semantic"
                profile = "deep"
                output = $output
                solc = (Resolve-Path -LiteralPath $Solc).Path
                timeout_seconds = 120
                jobs = 1
                compiler_jobs = 1
            }
        } | ConvertTo-Json -Depth 8 -Compress
        $response = $request | & $engine
        Assert-NativeSuccess "scopeth analyze $RequestId"
        $terminal = @($response | ForEach-Object { $_ | ConvertFrom-Json } | Where-Object { $_.type -in @("result", "error") })
        if ($terminal.Count -ne 1 -or $terminal[0].type -ne "result" -or $terminal[0].result.status -ne "complete") {
            throw "La integración real con solc no terminó como complete para $FixturePath."
        }
        return $terminal[0].result
    }

    $null = Invoke-ScopethAnalysis $fixture "validation-vault"
    $f4 = Invoke-ScopethAnalysis $fixtureF4 "validation-f4-cold"
    $hot = Invoke-ScopethAnalysis $fixtureF4 "validation-f4-hot"
    if (-not $hot.cache_reused) {
        throw "La segunda ejecución no reutilizó la caché verificada."
    }

    $summary = Get-Content -LiteralPath (Join-Path $f4.run_dir "summary.json") -Raw | ConvertFrom-Json
    if ($summary.counts.asset_operations -lt 20 -or $summary.counts.configuration_changes -lt 6 -or $summary.counts.assembly_operations -lt 8 -or $summary.counts.observations -lt 20) {
        throw "La cobertura F4 observada es inferior al contrato de regresión."
    }
    foreach ($name in @("manifest.json", "report.md", "configuration-changes.jsonl", "assembly-operations.jsonl")) {
        if (-not (Test-Path -LiteralPath (Join-Path $f4.run_dir $name) -PathType Leaf)) {
            throw "Falta resultado obligatorio: $name"
        }
    }

    foreach ($operation in @("query_flows", "query_surfaces", "query_configuration", "query_graph")) {
        $options = @{ limit = 10; offset = 0 }
        if ($operation -eq "query_graph") {
            $options = @{ kind = "assets"; format = "json"; depth = 4 }
        }
        $query = @{
            protocol_version = "1.0"; request_id = "validation-$operation"; operation = $operation
            run_dir = $f4.run_dir; options = $options
        } | ConvertTo-Json -Depth 6 -Compress
        $messages = $query | & $engine | ForEach-Object { $_ | ConvertFrom-Json }
        Assert-NativeSuccess "$operation"
        $queryTerminal = @($messages | Where-Object { $_.type -in @("result", "error") })
        if ($queryTerminal.Count -ne 1 -or $queryTerminal[0].type -ne "result") {
            throw "Falló la consulta de producción: $operation"
        }
    }
} finally {
    Pop-Location
    if (Test-Path -LiteralPath $output) {
        Remove-Item -LiteralPath $output -Recurse -Force
    }
}

Write-Host "Validación completa."
