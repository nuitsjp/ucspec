param([string]$Version = '0.1.0')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Invalid version.' }
    $info = & .\dist\ucspec.exe version --json | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or $info.version -ne $Version) { throw 'Run build.ps1 with the requested version first.' }
    foreach ($p in @('dist/third-party','dist/SOURCES.txt','dist/BUILD-INFO.txt')) { if (-not (Test-Path $p)) { throw "Missing distribution resource: $p" } }
    $stage = Join-Path ([IO.Path]::GetTempPath()) ('ucspec-package-' + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $stage | Out-Null
    try {
        Copy-Item dist/ucspec.exe,LICENSE,README.md,THIRD_PARTY_NOTICES.md -Destination $stage
        Copy-Item docs -Destination $stage -Recurse
        Copy-Item dist/third-party -Destination $stage -Recurse
        Copy-Item dist/SOURCES.txt,dist/BUILD-INFO.txt -Destination $stage
        $name = "ucspec_${Version}_windows_amd64.zip"
        $zip = Join-Path $root "dist/$name"
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip -Force
        $hash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
        [IO.File]::WriteAllText((Join-Path $root 'dist/checksums.txt'),"$hash  $name`n",[Text.UTF8Encoding]::new($false))
        Write-Host $zip
    } finally { Remove-Item $stage -Recurse -Force }
} finally { Pop-Location }
