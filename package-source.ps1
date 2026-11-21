$ErrorActionPreference = 'Stop'
$root = (Resolve-Path $PSScriptRoot).Path
$destination = [IO.Path]::GetFullPath((Join-Path $root '../vyro-source.zip'))
$files = [Collections.Generic.List[IO.FileInfo]]::new()
Get-ChildItem -LiteralPath $root -File | Where-Object { $_.Extension -in @('.md','.ps1') -or $_.Name -eq '.gitignore' } | ForEach-Object { $files.Add($_) }
$allowed = @{
    'backend'=@('.go','.mod','.sum')
    'frontend/app'=@('.tsx','.css')
    'database'=@('.sql')
    'docs'=@('.md')
    'benchmarks'=@('.md','.json','.csv')
    '.github'=@('.yml','.yaml')
}
foreach ($directory in $allowed.Keys) {
    Get-ChildItem -LiteralPath (Join-Path $root $directory) -File -Recurse | Where-Object { $_.Extension -in $allowed[$directory] } | ForEach-Object { $files.Add($_) }
}
foreach ($name in @('package.json','package-lock.json','tsconfig.json','next.config.ts')) { $files.Add((Get-Item -LiteralPath (Join-Path $root "frontend/$name"))) }
Add-Type -AssemblyName System.IO.Compression.FileSystem
$stream = [IO.File]::Open($destination,[IO.FileMode]::Create)
$archive = [IO.Compression.ZipArchive]::new($stream,[IO.Compression.ZipArchiveMode]::Create)
try {
    foreach ($file in ($files | Sort-Object FullName -Unique)) {
        $relative = $file.FullName.Substring($root.Length+1).Replace('\','/')
        if ($relative -match '(^|/)(\.local|node_modules|\.next|\.env)(/|$)') { throw "Excluded path reached package whitelist: $relative" }
        [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive,$file.FullName,$relative,[IO.Compression.CompressionLevel]::Optimal) | Out-Null
    }
} finally { $archive.Dispose();$stream.Dispose() }
Write-Host "Source archive created: $destination"
