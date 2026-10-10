$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    go test -count=1 ./internal/model ./internal/engine ./internal/compiler ./internal/analysis ./internal/project ./internal/update ./internal/report ./internal/app ./internal/assets
    if ($LASTEXITCODE -ne 0) { throw 'Core tests failed.' }
    Write-Host 'Core tests only. External adapters and actual ELK rendering require go test ./... .'
} finally { Pop-Location }
