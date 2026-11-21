$ErrorActionPreference = 'Stop'
$directory = Join-Path $PSScriptRoot 'benchmarks'
$culture = [Globalization.CultureInfo]::InvariantCulture
$lines = [Collections.Generic.List[string]]::new()
$lines.Add('# Reproducible HTTP benchmark results')
$lines.Add('')
$lines.Add('Run `./benchmark.ps1` from the project root to regenerate six reports against a separate database and API on port 8081. Each configuration uses 3 rounds of 100 measured requests, 10 excluded warmup requests, and a 12-driver fleet. Results below are local measurements, not production capacity estimates.')
$lines.Add('')
$lines.Add('| Scenario | Clients | Requests | Assigned | No driver | Errors | POST p50 | POST p95 | Details |')
$lines.Add('|---|---:|---:|---:|---:|---:|---:|---:|---|')
$reports = @()
foreach ($scenario in @('saturation','lifecycle','mixed')) {
    foreach ($concurrency in @(1,10)) {
        $name = "$scenario-c$concurrency"
        $path = Join-Path $directory "$name.json"
        $r = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
        $row = [string]::Format($culture,'| {0} | {1} | {2} | {3} | {4} | {5} | {6:F2} ms | {7:F2} ms | [Report]({8}.md) |',$scenario,$concurrency,$r.summary.requests,$r.summary.assigned,$r.summary.noDriver,$r.summary.errors,$r.summary.p50Ms,$r.summary.p95Ms,$name)
        $lines.Add($row)
        $reports += @{name=$name;createdAt=$r.createdAt;sha256=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLower()}
    }
}
$lines.Add('')
$lines.Add('POST timings include the HTTP exchange and decoding against the direct Go API, excluding Next.js. They exclude release calls, integrity snapshots, and warmup; failed requests would be included in percentiles. Total experiment time in each report includes cleanup and integrity checks. No requests-per-second capacity claim is made.')
$lines.Add('')
$lines.Add('Saturation and mixed demand hold reservations until the end of each round, then cancel only their own rides. With 12 drivers, most requests return no-driver, so low latency must not be read as high assignment capacity. Mixed demand also has distant pickups, but saturation masks much of that distinction. Lifecycle cancels each assignment before that worker sends its next request, allowing every central request to be matched.')
$lines.Add('')
$lines.Add('Database history and caches are retained between configurations. Concurrency 1 and 10 are different closed-loop workloads, not a controlled optimization comparison. The benchmark client and API share a machine and database cluster. Runtime/environment and raw per-request JSON/CSV are included in the linked reports. `manifest.json` fingerprints backend sources and report JSON files; the sources are the code used to generate these measurements.')
$lines | Set-Content -LiteralPath (Join-Path $directory 'README.md') -Encoding UTF8
$backend = Join-Path $PSScriptRoot 'backend'
$sourceFiles = @(Get-ChildItem -LiteralPath $backend -Filter '*.go' -Recurse) + @(Get-Item (Join-Path $backend 'go.mod'),(Join-Path $backend 'go.sum'))
$sources = @($sourceFiles | Sort-Object FullName | ForEach-Object { @{path=$_.FullName.Substring($PSScriptRoot.Length+1).Replace('\','/');sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower()} })
@{generatedAt=[DateTime]::UtcNow.ToString('o');backendSources=$sources;reports=$reports} | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $directory 'manifest.json') -Encoding UTF8
