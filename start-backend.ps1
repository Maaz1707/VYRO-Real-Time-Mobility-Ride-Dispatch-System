param([string]$GoPath = 'go', [switch]$Memory)
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot 'backend')
if ($Memory) { $env:DATABASE_URL = '' }
elseif (-not $env:DATABASE_URL -and (Test-Path (Join-Path $PSScriptRoot '.local/database.json'))) {
    & (Join-Path $PSScriptRoot 'start-database.ps1')
    $config = Get-Content (Join-Path $PSScriptRoot '.local/database.json') -Raw | ConvertFrom-Json
    $env:DATABASE_URL = "postgres://vyro_app:$($config.appPassword)@127.0.0.1:$($config.port)/vyro?sslmode=disable"
}
if (-not (Get-Command $GoPath -ErrorAction SilentlyContinue)) {
    $localGo = Join-Path $PSScriptRoot '../../work/tools/go/bin/go.exe'
    if (Test-Path $localGo) { $GoPath = (Resolve-Path $localGo).Path }
    else { throw 'Install Go, or supply -GoPath with the path to go.exe.' }
}
& $GoPath run .
if ($LASTEXITCODE -ne 0) { throw 'VYRO backend exited with an error.' }
