param([string]$RunName = ('agent-desktop-' + (Get-Date -Format 'yyyyMMdd-HHmmss')), [switch]$Compact, [string]$BinaryName = 'OpsBuddy.exe', [switch]$ImportDemo)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path $PSScriptRoot -Parent
Set-Location $workspaceRoot
if ($RunName -notmatch '^[a-zA-Z0-9_-]+$') { throw 'Use a simple unique run name.' }
if ($BinaryName -notmatch '^OpsBuddy(?:-[0-9.]+)?\.exe$') { throw 'Use an OpsBuddy executable name.' }
$runRoot = Join-Path $workspaceRoot ('.cache\' + $RunName)
if (Test-Path -LiteralPath $runRoot) { throw 'Use a new run directory; existing data must not be reused.' }
New-Item -ItemType Directory -Path $runRoot | Out-Null
if ($ImportDemo) {
    $imports = Join-Path $runRoot 'imports'
    New-Item -ItemType Directory -Path $imports | Out-Null
    Copy-Item -LiteralPath (Join-Path $workspaceRoot 'demos\multi-agent-todo\todo-demo.opsbuddy-project.json') -Destination $imports
}
$env:GOPATH = Join-Path $workspaceRoot '.cache\go'
$env:GOMODCACHE = Join-Path $workspaceRoot '.cache\gomod'
$env:GOCACHE = Join-Path $workspaceRoot '.cache\gobuild'
$env:TEMP = Join-Path $workspaceRoot '.cache\testtmp'
$env:TMP = $env:TEMP
$fixtureExe = Join-Path $workspaceRoot '.cache\agent-smoke-fixture.exe'
& go build -o $fixtureExe ./tools/agent-smoke-fixture
if ($LASTEXITCODE -ne 0) { throw 'Fixture build failed.' }
$urlFile = Join-Path $runRoot 'model-url.txt'
$fixture = $null
$desktop = $null
$priorURL = $env:OPSBUDDY_SMOKE_MODEL_URL
$priorImport = $env:OPSBUDDY_SMOKE_IMPORT_DEMO
try {
    if ($ImportDemo) { $env:OPSBUDDY_SMOKE_IMPORT_DEMO = '1' } else { $env:OPSBUDDY_SMOKE_IMPORT_DEMO = $null }
    $fixture = Start-Process -FilePath $fixtureExe -ArgumentList @('--url-file', ('"' + $urlFile + '"')) -WindowStyle Hidden -PassThru
    $deadline = (Get-Date).AddSeconds(10)
    while (-not (Test-Path -LiteralPath $urlFile)) {
        if ($fixture.HasExited -or (Get-Date) -gt $deadline) { throw 'Local fixture did not start.' }
        Start-Sleep -Milliseconds 100
    }
    $env:OPSBUDDY_SMOKE_MODEL_URL = [IO.File]::ReadAllText($urlFile).Trim()
    if ($Compact) { [IO.File]::WriteAllText((Join-Path $runRoot 'window.json'), '{"width":1000,"height":700}', (New-Object Text.UTF8Encoding($false))) }
    $desktopExe = Join-Path (Join-Path $workspaceRoot 'release\OpsBuddy') $BinaryName
    $desktop = Start-Process -FilePath $desktopExe -ArgumentList @('--data', ('"' + $runRoot + '"'), '--smoke-test') -WindowStyle Hidden -PassThru
    if (-not $desktop.WaitForExit(240000)) { throw 'Desktop self-check timed out.' }
    $reportFile = Join-Path $runRoot 'smoke-result.json'
    if (-not (Test-Path -LiteralPath $reportFile)) { throw 'Desktop exited without a self-check report.' }
    $report = Get-Content -LiteralPath $reportFile -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $report.passed -or ($ImportDemo -and -not $report.projectImport.goHealth) -or (-not $ImportDemo -and -not $report.projectAgentCollaboration.controls.stopGroup)) {
        throw ('Desktop self-check failed: ' + $report.phase + ' / ' + $report.error)
    }
    $receipts = Invoke-RestMethod -Uri ($env:OPSBUDDY_SMOKE_MODEL_URL + '/evidence')
    ConvertTo-Json -InputObject $receipts -Depth 8 | Set-Content -LiteralPath (Join-Path $runRoot 'model-receipts.json') -Encoding UTF8
    $evidence = [ordered]@{
        passed = $true
        exeVersion = (Get-Item -LiteralPath $desktopExe).VersionInfo.FileVersion
        exeSHA256 = (Get-FileHash -LiteralPath $desktopExe -Algorithm SHA256).Hash
        finishedAt = (Get-Date).ToUniversalTime().ToString('o')
        report = $reportFile
        isolatedData = $runRoot
        mockRequests = @($receipts).Count
        compact = [bool]$Compact
        importDemo = [bool]$ImportDemo
    }
    $evidence | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $runRoot 'verification.json') -Encoding UTF8
    $evidence | ConvertTo-Json
} finally {
    if ($desktop -and -not $desktop.HasExited) {
        [void]$desktop.CloseMainWindow()
        if (-not $desktop.WaitForExit(10000)) {
            Write-Warning ('Isolated test desktop still running, PID ' + $desktop.Id)
        }
    }
    if ($fixture -and -not $fixture.HasExited) { Stop-Process -Id $fixture.Id }
    $env:OPSBUDDY_SMOKE_MODEL_URL = $priorURL
    $env:OPSBUDDY_SMOKE_IMPORT_DEMO = $priorImport
}
