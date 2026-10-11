param(
    [string]$Repository = 'nuitsjp/ucspec',
    [ValidateSet('private','public')][string]$Visibility = 'private'
)
$ErrorActionPreference = 'Stop'
if ($Repository -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$' -or $Repository.Contains('..')) { throw 'Invalid repository.' }
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    Get-Command git,gh,go -ErrorAction Stop | Out-Null
    gh auth status
    if ($LASTEXITCODE -ne 0) { throw 'Authenticate with gh auth login first.' }
    # Read before write. Never overwrite or push into an existing repository.
    gh repo view $Repository --json nameWithOwner 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { throw 'Repository already exists; this script only creates a NEW repository.' }
    & .\scripts\build.ps1 -Repository $Repository
    if (-not (Test-Path go.sum)) { throw 'Missing dependency lock.' }
    if (-not (Test-Path .git)) { git init -b main; if ($LASTEXITCODE -ne 0) { throw 'git init failed.' } }
    $origin = git remote get-url origin 2>$null
    if ($LASTEXITCODE -eq 0) { throw "Existing origin found ($origin); refusing to repoint it." }
    $identity = gh api user | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or -not $identity.login) { throw 'Unable to resolve GitHub identity.' }
    git add --all
    if ($LASTEXITCODE -ne 0) { throw 'git add failed.' }
    git diff --cached --quiet
    if ($LASTEXITCODE -ne 0) {
        $email = "$($identity.id)+$($identity.login)@users.noreply.github.com"
        git -c "user.name=$($identity.login)" -c "user.email=$email" commit -m 'Initial ucspec implementation, tests, documentation and sample'
        if ($LASTEXITCODE -ne 0) { throw 'Initial commit failed.' }
    }
    gh repo create $Repository "--$Visibility" --source . --remote origin --push --description 'Structured use-case specifications, finite model checks, coverage and standalone HTML'
    if ($LASTEXITCODE -ne 0) { throw 'Repository creation/push failed. No existing remote was overwritten.' }
    Write-Host "Created https://github.com/$Repository"
    Write-Host 'Review Windows CI before creating a version tag and Release.'
} finally { Pop-Location }
