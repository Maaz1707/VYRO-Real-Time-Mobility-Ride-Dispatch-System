param([string]$GoPath = 'go', [switch]$Database)
$ErrorActionPreference = 'Stop'
if (-not (Get-Command $GoPath -ErrorAction SilentlyContinue)) {
    $GoPath = (Resolve-Path (Join-Path $PSScriptRoot '../../work/tools/go/bin/go.exe')).Path
}
$previous = $env:VYRO_TEST_DATABASE_URL
try {
    if ($Database) {
        & (Join-Path $PSScriptRoot 'start-database.ps1')
        $config = Get-Content (Join-Path $PSScriptRoot '.local/database.json') -Raw | ConvertFrom-Json
        $env:VYRO_TEST_DATABASE_URL = "postgres://vyro_test:$($config.testPassword)@127.0.0.1:$($config.port)/vyro_test?sslmode=disable"
    }
    Set-Location (Join-Path $PSScriptRoot 'backend')
    & $GoPath test -v -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed.' }
    & $GoPath vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
} finally { $env:VYRO_TEST_DATABASE_URL = $previous }
