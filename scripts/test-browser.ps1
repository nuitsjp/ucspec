param(
    [string]$Report = 'examples/customer-crud/artifacts/ucspec/index.html',
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
$work = Join-Path ([IO.Path]::GetTempPath()) ('ucspec-browser-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
# Build isolated review fixtures through the production CLI and real layout adapter.
$exe = Join-Path $root 'dist/ucspec.exe'
if (-not (Test-Path -LiteralPath $exe)) { throw 'Run scripts/build.ps1 before the browser test.' }
$fixture = Join-Path $work 'review-product'
New-Item -ItemType Directory -Path $fixture | Out-Null
Copy-Item -LiteralPath (Join-Path $root 'examples/customer-crud/spec') -Destination $fixture -Recurse
$project = Join-Path $fixture 'spec/project.json'
& $exe build --project $project | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Review fixture baseline failed.' }
$fixtureOutput = Join-Path $fixture 'artifacts/ucspec'
$baseline = [IO.File]::ReadAllText((Join-Path $fixtureOutput 'report.json'), $utf8) | ConvertFrom-Json
$usecase = $baseline.usecases | Where-Object id -eq 'viewCustomer'
$missing = @(($usecase.coverage | Where-Object kind -eq 'boundedPaths').missing)
if ($missing.Count -lt 5) { throw 'Review fixture requires at least five missing paths.' }
$source = Join-Path $fixture 'spec/usecases/viewCustomer.json'
$document = [IO.File]::ReadAllText($source, $utf8) | ConvertFrom-Json
$reviews = [ordered]@{}
foreach ($status in @('addScenario','represented','forbidden')) {
    $index = $reviews.Count
    $review = [ordered]@{
        kind = 'boundedPaths'
        requirements = @($missing[$index].id)
        status = $status
        reason = "Browser fixture: $status <script>window.injected=true</script>"
        basisHash = $usecase.reviewBasisHash
    }
    if ($status -eq 'represented') {
        $review.scenario = 'view'
        $review.architectureRequirements = @('The represented paths must preserve the same user outcome; fixture requirement, not an implementation guarantee.')
        $review.requirements += $missing[3].id
    }
    $reviews[$status] = $review
}
$document.usecases.viewCustomer | Add-Member -NotePropertyName coverageReviews -NotePropertyValue $reviews -Force
[IO.File]::WriteAllText($source, ($document | ConvertTo-Json -Depth 100), $utf8)
& $exe build --project $project | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Review fixture build failed.' }
$reviewHTML = $utf8.GetString([IO.File]::ReadAllBytes((Join-Path $fixtureOutput 'index.html')))
# Change intent while preserving paths: historical judgments must become stale.
$document.usecases.viewCustomer.scenarios.view.goal += ' (changed review context)'
[IO.File]::WriteAllText($source, ($document | ConvertTo-Json -Depth 100), $utf8)
& $exe build --project $project | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Stale review fixture build failed.' }
$inputs = @{
    output = $Output.Replace("\", "/")
    report = $utf8.GetString([IO.File]::ReadAllBytes($Report))
    sample = $utf8.GetString([IO.File]::ReadAllBytes((Join-Path $root 'examples/customer-crud/app/index.html')))
    reviews = $reviewHTML
    stale = $utf8.GetString([IO.File]::ReadAllBytes((Join-Path $fixtureOutput 'index.html')))
} | ConvertTo-Json -Compress
$code = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'browser-smoke.js'), $utf8).Replace('__INPUTS__', $inputs)
[IO.File]::WriteAllText((Join-Path $work 'smoke.js'), $code, $utf8)
$session = 'ucspec-' + [guid]::NewGuid().ToString('N')
# `open` starts a session daemon that inherits the caller's standard handles. A
# pipe would therefore never reach end-of-file, so output goes to a new file per
# call (the daemon keeps the handle of the `open` output open), which is read with
# sharing allowed. Only the CLI process itself is awaited; Start-Process -Wait
# would also await the daemon.
$cliCall = 0
function Invoke-PlaywrightCli([string[]]$Arguments) {
    $script:cliCall++
    $stdout = Join-Path $work "cli-$($script:cliCall)-stdout.txt"
    $stderr = Join-Path $work "cli-$($script:cliCall)-stderr.txt"
    $quoted = @($cli) + $Arguments | ForEach-Object { '"' + $_ + '"' }
    $process = Start-Process -FilePath 'node' -ArgumentList $quoted -WorkingDirectory $work -NoNewWindow -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    $process.WaitForExit()
    $stream = [IO.File]::Open($stdout, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite)
    try { $text = [IO.StreamReader]::new($stream, $utf8).ReadToEnd() } finally { $stream.Dispose() }
    [pscustomobject]@{ ExitCode = $process.ExitCode; Output = ($text | ConvertFrom-Json) }
}
$result = [pscustomobject]@{ passed = $false; checks = @(); errors = @() }
Push-Location $work
try {
    $opened = Invoke-PlaywrightCli @("-s=$session", 'open', 'about:blank', '--browser=msedge', '--json')
    if ($opened.ExitCode -ne 0 -or $opened.Output.isError) { throw "Browser startup failed: $($opened.Output.error)" }
    $run = Invoke-PlaywrightCli @("-s=$session", 'run-code', '--filename=smoke.js', '--json')
    # playwright-cli can return exit code 0 together with isError=true.
    if ($run.ExitCode -ne 0 -or $run.Output.isError -or -not $run.Output.result) { throw "Browser test execution failed: $($run.Output.error)" }
    $result = $run.Output.result | ConvertFrom-Json
} catch {
    $result.passed = $false
    $result.errors += $_.Exception.Message
} finally {
    try {
        $closed = Invoke-PlaywrightCli @("-s=$session", 'close', '--json')
        if ($closed.ExitCode -ne 0 -or $closed.Output.isError) { throw "Browser cleanup failed: $($closed.Output.error)" }
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
