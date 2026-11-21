$ErrorActionPreference = 'Stop'
$local = Join-Path $PSScriptRoot '.local'
$configPath = Join-Path $local 'database.json'
if (-not (Test-Path $configPath)) { throw 'Run setup-database.ps1 first.' }
$control = Join-Path $local 'postgres/bin/pg_ctl.exe'
$data = Join-Path $local 'pgdata'
& $control -D $data status | Out-Null
if ($LASTEXITCODE -ne 0) {
    & $control -D $data -l (Join-Path $local 'postgres.log') -w start
    if ($LASTEXITCODE -ne 0) { throw 'Could not start the VYRO database. Inspect .local/postgres.log.' }
}
