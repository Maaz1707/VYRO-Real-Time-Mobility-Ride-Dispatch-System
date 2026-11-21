$ErrorActionPreference = 'Stop'
$local = Join-Path $PSScriptRoot '.local'
$control = Join-Path $local 'postgres/bin/pg_ctl.exe'
& $control -D (Join-Path $local 'pgdata') -w -m fast stop
if ($LASTEXITCODE -ne 0) { throw 'Could not stop the VYRO database.' }
