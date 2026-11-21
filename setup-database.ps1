param(
    [string]$PostgreSQLHome = 'C:\Program Files\PostgreSQL\17',
    [string]$PostGISArchive = '',
    [int]$Port = 55432
)
$ErrorActionPreference = 'Stop'
$local = Join-Path $PSScriptRoot '.local'
$runtime = Join-Path $local 'postgres'
$data = Join-Path $local 'pgdata'
$configPath = Join-Path $local 'database.json'
New-Item -ItemType Directory -Force $local | Out-Null

if (-not (Test-Path (Join-Path $runtime 'bin/initdb.exe'))) {
    if (-not (Test-Path (Join-Path $PostgreSQLHome 'bin/initdb.exe'))) { throw 'PostgreSQL 17 binaries were not found. Supply -PostgreSQLHome.' }
    New-Item -ItemType Directory -Force $runtime | Out-Null
    foreach ($folder in @('bin','lib','share')) { Copy-Item -LiteralPath (Join-Path $PostgreSQLHome $folder) -Destination $runtime -Recurse -Force }
}
if (-not (Test-Path (Join-Path $runtime 'share/extension/postgis.control'))) {
    $downloads = Join-Path $local 'downloads'
    New-Item -ItemType Directory -Force $downloads | Out-Null
    if (-not $PostGISArchive) {
        $PostGISArchive = Join-Path $downloads 'postgis.zip'
        $url = 'https://download-cache.osgeo.org/postgis/windows/pg17/postgis-bundle-pg17-3.7.0x64.zip'
        Write-Host 'Downloading official PostGIS binaries for PostgreSQL 17...'
        Invoke-WebRequest $url -OutFile $PostGISArchive
        $checksumFile = Join-Path $downloads 'postgis.md5'
        Invoke-WebRequest ($url + '.md5') -OutFile $checksumFile
        $expected = ((Get-Content -LiteralPath $checksumFile -Raw).Trim() -split '\s+')[0]
        if ((Get-FileHash $PostGISArchive -Algorithm MD5).Hash.ToLower() -ne $expected.ToLower()) { throw 'PostGIS archive checksum mismatch.' }
    }
    $expanded = Join-Path $downloads 'postgis'
    Expand-Archive -LiteralPath $PostGISArchive -DestinationPath $expanded -Force
    $control = Get-ChildItem $expanded -Filter postgis.control -Recurse | Select-Object -First 1
    if (-not $control) { throw 'Archive does not contain postgis.control.' }
    $bundle = $control.Directory.Parent.Parent.FullName
    foreach ($folder in @('bin','lib','share')) {
        Get-ChildItem -LiteralPath (Join-Path $bundle $folder) | Copy-Item -Destination (Join-Path $runtime $folder) -Recurse -Force
    }
}
# The 3.7 Windows ZIP leaves Brotli under postgisgui and omits nghttp2.
# Reuse matching MinGW runtime DLLs from an existing Git for Windows install.
# Copy only into the project runtime; no system PATH or DLLs are changed.
foreach ($name in @('libbrotlicommon.dll','libbrotlidec.dll','libnghttp2-14.dll')) {
    $destination = Join-Path $runtime "bin/$name"
    if (-not (Test-Path $destination)) {
        $candidates = @((Join-Path $runtime "bin/postgisgui/$name"),(Join-Path $env:ProgramFiles "Git/mingw64/bin/$name"))
        $source = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
        if (-not $source) { throw "Missing $name. Install Git for Windows to supply the MinGW dependency, then rerun setup." }
        Copy-Item -LiteralPath $source -Destination $destination
    }
}
if (-not (Test-Path $configPath)) {
    function New-LocalPassword {
        $bytes = New-Object byte[] 32
        $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
        try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
        return ([BitConverter]::ToString($bytes)).Replace('-','').ToLower()
    }
    @{port=$Port;adminPassword=(New-LocalPassword);appPassword=(New-LocalPassword);testPassword=(New-LocalPassword)} | ConvertTo-Json | Set-Content -LiteralPath $configPath -Encoding UTF8
}
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
if (-not $config.benchmarkPassword) {
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    $benchmarkPassword = ([BitConverter]::ToString($bytes)).Replace('-','').ToLower()
    $config | Add-Member -NotePropertyName benchmarkPassword -NotePropertyValue $benchmarkPassword
    $config | ConvertTo-Json | Set-Content -LiteralPath $configPath -Encoding UTF8
}
if (-not (Test-Path (Join-Path $data 'PG_VERSION'))) {
    $pwFile = Join-Path $local 'init-password.tmp'
    try {
        Set-Content -LiteralPath $pwFile -Value $config.adminPassword -Encoding ASCII
        & (Join-Path $runtime 'bin/initdb.exe') -D $data -U vyro_admin --pwfile=$pwFile --auth-host=scram-sha-256 --auth-local=scram-sha-256 --encoding=UTF8 --locale=C
        if ($LASTEXITCODE -ne 0) { throw 'Database initialization failed.' }
    } finally { if (Test-Path $pwFile) { Remove-Item -LiteralPath $pwFile } }
    @"

# VYRO project database, separate from the system PostgreSQL service.
listen_addresses = '127.0.0.1'
port = $($config.port)
max_connections = 40
shared_buffers = '64MB'
"@ | Add-Content -LiteralPath (Join-Path $data 'postgresql.conf')
}
& (Join-Path $PSScriptRoot 'start-database.ps1')
$oldPassword = $env:PGPASSWORD
try {
    $env:PGPASSWORD = $config.adminPassword
    $psql = Join-Path $runtime 'bin/psql.exe'
    $argsBase = @('-w','-h','127.0.0.1','-p',"$($config.port)",'-U','vyro_admin','-v','ON_ERROR_STOP=1')
    foreach ($role in @(@{name='vyro_app';password=$config.appPassword},@{name='vyro_test';password=$config.testPassword},@{name='vyro_bench';password=$config.benchmarkPassword})) {
        $exists = & $psql @argsBase -d postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='$($role.name)'"
        if ($LASTEXITCODE -ne 0) { throw 'Could not inspect database roles.' }
        if ($exists -ne '1') {
            "CREATE ROLE $($role.name) LOGIN PASSWORD '$($role.password)';" | & $psql @argsBase -d postgres
            if ($LASTEXITCODE -ne 0) { throw 'Could not create project database role.' }
        }
    }
    foreach ($db in @(@{name='vyro';role='vyro_app'},@{name='vyro_test';role='vyro_test'},@{name='vyro_bench';role='vyro_bench'})) {
        $exists = & $psql @argsBase -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$($db.name)'"
        if ($LASTEXITCODE -ne 0) { throw 'Could not inspect databases.' }
        if ($exists -ne '1') {
            & (Join-Path $runtime 'bin/createdb.exe') -w -h 127.0.0.1 -p $config.port -U vyro_admin $db.name
            if ($LASTEXITCODE -ne 0) { throw 'Could not create project database.' }
        }
        & $psql @argsBase -d $db.name -f (Join-Path $PSScriptRoot 'database/schema.sql')
        if ($LASTEXITCODE -ne 0) { throw 'Schema migration failed.' }
        $privileges = 'SELECT,INSERT,UPDATE,DELETE'
        if ($db.role -eq 'vyro_test') { $privileges += ',TRUNCATE' }
        "GRANT CONNECT ON DATABASE $($db.name) TO $($db.role); GRANT USAGE ON SCHEMA public TO $($db.role); GRANT $privileges ON ALL TABLES IN SCHEMA public TO $($db.role);" | & $psql @argsBase -d $db.name
        if ($LASTEXITCODE -ne 0) { throw 'Could not grant project database access.' }
    }
    & $psql @argsBase -d vyro -c 'SELECT PostGIS_Version();'
} finally { $env:PGPASSWORD = $oldPassword }
Write-Host "VYRO database ready on 127.0.0.1:$($config.port). Local credentials are in ignored .local/database.json."
