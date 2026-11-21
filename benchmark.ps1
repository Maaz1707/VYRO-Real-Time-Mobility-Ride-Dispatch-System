param([string]$GoPath = 'go', [int]$Port = 8081)
$ErrorActionPreference = 'Stop'
if (-not (Get-Command $GoPath -ErrorAction SilentlyContinue)) { $GoPath = (Resolve-Path (Join-Path $PSScriptRoot '../../work/tools/go/bin/go.exe')).Path }
if (-not (Test-Path (Join-Path $PSScriptRoot '.local/database.json'))) { throw 'Run setup-database.ps1 first.' }
$config = Get-Content (Join-Path $PSScriptRoot '.local/database.json') -Raw | ConvertFrom-Json
if (-not $config.benchmarkPassword) {
    & (Join-Path $PSScriptRoot 'setup-database.ps1')
    $config = Get-Content (Join-Path $PSScriptRoot '.local/database.json') -Raw | ConvertFrom-Json
}
& (Join-Path $PSScriptRoot 'start-database.ps1')
Set-Location (Join-Path $PSScriptRoot 'backend')
$binary = Join-Path $PSScriptRoot '.local/benchmark-api.exe'
$runner = Join-Path $PSScriptRoot '.local/benchmark-client.exe'
& $GoPath build -o $binary .
if ($LASTEXITCODE -ne 0) { throw 'API compilation failed.' }
& $GoPath build -o $runner ./cmd/benchmark
if ($LASTEXITCODE -ne 0) { throw 'Benchmark client compilation failed.' }
$previousURL=$env:DATABASE_URL
$previousAddress=$env:VYRO_ADDR
$apiProcess=$null
try {
    $env:DATABASE_URL="postgres://vyro_bench:$($config.benchmarkPassword)@127.0.0.1:$($config.port)/vyro_bench?sslmode=disable"
    $env:VYRO_ADDR="127.0.0.1:$Port"
    $apiProcess=Start-Process -FilePath $binary -WorkingDirectory $PSScriptRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $PSScriptRoot '.local/benchmark-access.log') -RedirectStandardError (Join-Path $PSScriptRoot '.local/benchmark-api.log')
    $ready=$false
    for ($attempt=0;$attempt -lt 30;$attempt++) {
        $apiProcess.Refresh()
        if ($apiProcess.HasExited) { throw 'Benchmark API failed to start. Check .local/benchmark-api.log; the port may already be in use.' }
        try { $health=Invoke-RestMethod "http://127.0.0.1:$Port/api/health" -TimeoutSec 2; if ($health.storage -eq 'postgres') { $ready=$true;break } } catch { Start-Sleep -Milliseconds 250 }
    }
    if (-not $ready) { throw 'Benchmark API readiness timed out.' }
    $directory=Join-Path $PSScriptRoot 'benchmarks'
    New-Item -ItemType Directory -Force $directory | Out-Null
    foreach ($scenario in @('saturation','lifecycle','mixed')) {
        foreach ($concurrency in @(1,10)) {
            $output=Join-Path $directory "$scenario-c$concurrency.json"
            & $runner -url "http://127.0.0.1:$Port" -scenario $scenario -requests 100 -rounds 3 -warmup 10 -concurrency $concurrency -out $output
            if ($LASTEXITCODE -ne 0) { throw "Benchmark $scenario / concurrency $concurrency failed. Inspect its report and API logs." }
        }
    }
    & (Join-Path $PSScriptRoot 'summarize-benchmarks.ps1')
} finally {
    if ($apiProcess -and -not $apiProcess.HasExited) { Stop-Process -Id $apiProcess.Id; $apiProcess.WaitForExit() }
    $env:DATABASE_URL=$previousURL
    $env:VYRO_ADDR=$previousAddress
}
