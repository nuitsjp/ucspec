param(
    [string]$Version = '0.1.0',
    [string]$Repository = 'nuitsjp/ucspec'
)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'This release build script targets Windows amd64.' }
if ($Version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Version must be x.y.z.' }
if ($Repository -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$' -or $Repository.Contains('..')) { throw 'Invalid repository.' }
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
$previousCGO = $env:CGO_ENABLED
$previousOS = $env:GOOS
$previousArch = $env:GOARCH
try {
    Get-Command go -ErrorAction Stop | Out-Null
    foreach ($pair in @(@('docs/authoring.md','internal/assets/authoring.md'),@('docs/user-manual.md','internal/assets/manual.md'))) {
        if ((Get-FileHash $pair[0]).Hash -ne (Get-FileHash $pair[1]).Hash) { throw "Embedded guide differs: $($pair[0])" }
    }
    go mod tidy
    if ($LASTEXITCODE -ne 0) { throw 'Dependency restore failed. No binaries were released.' }
    go mod download all
    if ($LASTEXITCODE -ne 0) { throw 'Dependency source download failed.' }
    go mod verify
    if ($LASTEXITCODE -ne 0) { throw 'Module verification failed.' }
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed. Release build aborted.' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
    New-Item -ItemType Directory -Force -Path dist | Out-Null
    $ldflags = "-s -w -X github.com/nuitsjp/ucspec/internal/buildinfo.Version=$Version -X github.com/nuitsjp/ucspec/internal/buildinfo.Repository=$Repository"
    go build -mod=readonly -trimpath -ldflags $ldflags -o dist/ucspec.exe ./cmd/ucspec
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    $info = & .\dist\ucspec.exe version --json | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or $info.version -ne $Version) { throw 'Built executable did not pass version probe.' }
    & .\dist\ucspec.exe build --project examples/customer-crud/spec/project.json
    if ($LASTEXITCODE -ne 0) { throw 'Actual CLI sample build failed.' }
    $notice = Join-Path $root 'dist/third-party'
    if (Test-Path $notice) { Remove-Item $notice -Recurse -Force }
    New-Item -ItemType Directory -Path $notice | Out-Null
    $sources = [System.Collections.Generic.List[string]]::new()
    $sources.Add('Dependency source availability for this build:')
    $modules = @(go list -m -f '{{.Path}}|{{.Version}}|{{.Dir}}' all)
    if ($LASTEXITCODE -ne 0) { throw 'Module listing failed.' }
    foreach ($line in $modules) {
        $parts = $line -split '\|', 3
        if ($parts.Count -lt 3 -or -not $parts[1]) { continue }
        if (-not $parts[2] -or -not (Test-Path -LiteralPath $parts[2])) { throw "Module sources missing: $line" }
        $slug = ($parts[0] -replace '[/\\:]','_') + '@' + $parts[1]
        $dest = Join-Path $notice $slug
        New-Item -ItemType Directory -Path $dest | Out-Null
        $licenses = @(Get-ChildItem -LiteralPath $parts[2] -File | Where-Object Name -Match '^(LICENSE|LICENCE|COPYING|NOTICE)([._-].*)?$')
        if ($licenses.Count -eq 0) { throw "Review missing license files for $($parts[0]) before distribution." }
        foreach ($file in $licenses) { Copy-Item -LiteralPath $file.FullName -Destination $dest }
        $sources.Add("$($parts[0]) $($parts[1])")
        $sources.Add("  go mod download -json $($parts[0])@$($parts[1])")
        $sources.Add("  https://pkg.go.dev/$($parts[0])@$($parts[1])")
    }
    $goRoot = (go env GOROOT).Trim()
    Copy-Item -LiteralPath (Join-Path $goRoot 'LICENSE') -Destination (Join-Path $notice 'Go-LICENSE')
    [IO.File]::WriteAllLines((Join-Path $root 'dist/SOURCES.txt'),$sources,[Text.UTF8Encoding]::new($false))
    $buildInfo = @("ucspec $Version", "Repository $Repository", (go version), (go version -m dist/ucspec.exe))
    [IO.File]::WriteAllLines((Join-Path $root 'dist/BUILD-INFO.txt'),$buildInfo,[Text.UTF8Encoding]::new($false))
    Write-Host 'Built and tested: dist/ucspec.exe'
    Write-Host 'Commit the generated go.sum before tagging a release.'
} finally {
    $env:CGO_ENABLED = $previousCGO; $env:GOOS = $previousOS; $env:GOARCH = $previousArch
    Pop-Location
}
