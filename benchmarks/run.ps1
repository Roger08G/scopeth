[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Solc,
    [ValidateRange(1, 100)]
    [int]$Iterations = 5,
    [string]$Output = "benchmarks\observed.json"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$engine = Join-Path $root "bin\scopeth-engine.exe"
if (-not (Test-Path -LiteralPath $engine -PathType Leaf)) {
    throw "Compila primero el motor con scripts\build.ps1."
}
if (-not (Test-Path -LiteralPath $Solc -PathType Leaf)) {
    throw "solc no existe: $Solc"
}

$corpora = @(
    @{ name = "small"; path = (Join-Path $root "fixtures\standard-json\simple.json") },
    @{ name = "medium"; path = (Join-Path $root "fixtures\solidity") },
    @{ name = "advanced"; path = (Join-Path $root "fixtures\advanced") }
)
$measurements = @()

foreach ($corpus in $corpora) {
    $resultRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("scopeth-bench-" + [guid]::NewGuid())
    try {
        for ($iteration = 1; $iteration -le $Iterations; $iteration++) {
            $request = @{
                protocol_version = "1.0"
                request_id = "benchmark-$iteration"
                operation = "analyze"
                input = $corpus.path
                options = @{
                    mode = "semantic"
                    profile = "deep"
                    output = $resultRoot
                    solc = (Resolve-Path -LiteralPath $Solc).Path
                    timeout_seconds = 300
                    jobs = 1
                    compiler_jobs = 1
                    refresh = ($iteration -eq 1)
                }
            } | ConvertTo-Json -Depth 8 -Compress
            $watch = [System.Diagnostics.Stopwatch]::StartNew()
            $response = $request | & $engine
            $watch.Stop()
            $terminal = $response | ForEach-Object { $_ | ConvertFrom-Json } | Where-Object { $_.type -eq "result" }
            if ($null -eq $terminal) {
                throw "El benchmark no recibió resultado terminal."
            }
            $size = (Get-ChildItem -LiteralPath $terminal.result.run_dir -File | Measure-Object -Property Length -Sum).Sum
            $measurements += @{
                corpus = $corpus.name
                iteration = $iteration
                elapsed_ms = $watch.ElapsedMilliseconds
                result_bytes = $size
                status = $terminal.result.status
                cache_reused = $terminal.result.cache_reused
                cache_state = if ($terminal.result.cache_reused) { "hot" } else { "cold" }
                engine_memory_bytes = "unavailable"
                compiler_memory_bytes = "unavailable"
            }
        }
    } finally {
        if (Test-Path -LiteralPath $resultRoot) {
            Remove-Item -LiteralPath $resultRoot -Recurse -Force
        }
    }
}

$payload = @{
    generated_at = [DateTimeOffset]::UtcNow.ToString("O")
    machine = $env:COMPUTERNAME
    os = [System.Environment]::OSVersion.VersionString
    processor_count = [System.Environment]::ProcessorCount
    dotnet_runtime = [System.Environment]::Version.ToString()
    solc = (& $Solc --version | Out-String).Trim()
    measurements = $measurements
}
$destination = Join-Path $root $Output
$parent = Split-Path -Parent $destination
New-Item -ItemType Directory -Force -Path $parent | Out-Null
$payload | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $destination -Encoding utf8NoBOM
Write-Host "Benchmark escrito en $destination"
