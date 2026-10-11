param(
    [string]$Report = 'examples/customer-crud/artifacts/usecase-spec/index.html',
    [string]$Output = 'validation/browser'
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$cli = Join-Path $root 'node_modules/@playwright/cli/playwright-cli.js'
if (-not (Test-Path -LiteralPath $cli)) { throw 'Run npm ci before the browser test.' }
if (-not [IO.Path]::IsPathRooted($Report)) { $Report = Join-Path $root $Report }
if (-not [IO.Path]::IsPathRooted($Output)) { $Output = Join-Path $root $Output }
$Report = (Resolve-Path -LiteralPath $Report).Path
New-Item -ItemType Directory -Force -Path $Output | Out-Null
$utf8 = [Text.UTF8Encoding]::new($false)
# Read bytes directly: newline normalization would hide CSP hash failures.
$inputs = @{
    report = $utf8.GetString([IO.File]::ReadAllBytes($Report))
    sample = $utf8.GetString([IO.File]::ReadAllBytes((Join-Path $root 'examples/customer-crud/app/index.html')))
} | ConvertTo-Json -Compress
$code = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'browser-smoke.js'), $utf8).Replace('__INPUTS__', $inputs)
$work = Join-Path ([IO.Path]::GetTempPath()) ('ucspec-browser-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
[IO.File]::WriteAllText((Join-Path $work 'smoke.js'), $code, $utf8)
$session = 'ucspec-' + [guid]::NewGuid().ToString('N')
$result = [pscustomobject]@{ passed = $false; checks = @(); errors = @() }
Push-Location $work
try {
    $opened = (& node $cli "-s=$session" open about:blank --browser=msedge --json | Out-String) | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or $opened.isError) { throw "Browser startup failed: $($opened.error)" }
    $run = (& node $cli "-s=$session" run-code --filename=smoke.js --json | Out-String) | ConvertFrom-Json
    # playwright-cli can return exit code 0 together with isError=true.
    if ($LASTEXITCODE -ne 0 -or $run.isError -or -not $run.result) { throw "Browser test execution failed: $($run.error)" }
    $result = $run.result | ConvertFrom-Json
} catch {
    $result.passed = $false
    $result.errors += $_.Exception.Message
} finally {
    try {
        $closed = (& node $cli "-s=$session" close --json | Out-String) | ConvertFrom-Json
        if ($LASTEXITCODE -ne 0 -or $closed.isError) { throw "Browser cleanup failed: $($closed.error)" }
    } catch {
        $result.passed = $false
        $result.errors += $_.Exception.Message
    } finally {
        Pop-Location
    }
    $result | Add-Member -NotePropertyName cliVersion -NotePropertyValue '0.1.11'
    $result | Add-Member -NotePropertyName method -NotePropertyValue 'UTF-8 bytes passed to setContent with original newlines; not file:// verification'
    $result | Add-Member -NotePropertyName cliLogs -NotePropertyValue $work
    [IO.File]::WriteAllText((Join-Path $Output 'results.json'), ($result | ConvertTo-Json -Depth 5), $utf8)
}
if (-not $result.passed) { throw ($result.errors -join [Environment]::NewLine) }
Write-Host "Browser smoke passed: $($result.checks.Count) checks. Results: $(Join-Path $Output 'results.json')"
