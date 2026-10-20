param([switch]$SkipFrontend, [string]$BinaryName = 'OpsBuddy.exe', [switch]$Package)
$ErrorActionPreference = 'Stop'
if ($BinaryName -notmatch '^OpsBuddy(?:-[0-9.]+)?\.exe$') { throw 'Use an OpsBuddy executable name.' }
$workspaceRoot = Split-Path $PSScriptRoot -Parent
Set-Location $workspaceRoot
$env:GOPATH = Join-Path $workspaceRoot '.cache\go'
$env:GOMODCACHE = Join-Path $workspaceRoot '.cache\gomod'
$env:GOCACHE = Join-Path $workspaceRoot '.cache\gobuild'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
if (-not $SkipFrontend) {
    if (-not (Test-Path -LiteralPath 'node_modules')) {
        & npm.cmd ci --cache .cache/npm
        if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
    }
    & npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
}
if (-not (Test-Path -LiteralPath 'dist\index.html')) { throw 'Build the frontend first.' }
$embedRoot = Join-Path $workspaceRoot 'cmd\opsbuddy-desktop\frontend'
New-Item -ItemType Directory -Force -Path $embedRoot | Out-Null
$generatedAssets = [IO.Path]::GetFullPath((Join-Path $embedRoot 'assets'))
if (Test-Path -LiteralPath $generatedAssets) {
    $generatedEntry = Get-Item -LiteralPath $generatedAssets
    if (-not $generatedAssets.StartsWith($workspaceRoot + '\', [StringComparison]::OrdinalIgnoreCase) -or ($generatedEntry.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing to clean assets outside the generated workspace directory.' }
    Remove-Item -LiteralPath $generatedAssets -Recurse -Force
}
Copy-Item -Path 'dist\*' -Destination $embedRoot -Recurse -Force
$releaseRoot = Join-Path $workspaceRoot 'release\OpsBuddy'
New-Item -ItemType Directory -Force -Path $releaseRoot | Out-Null
& go run ./tools/desktop-resource
if ($LASTEXITCODE -ne 0) { throw 'Windows resources could not be generated.' }
& go build -trimpath -tags production -ldflags '-s -w -H windowsgui' -o (Join-Path $releaseRoot $BinaryName) ./cmd/opsbuddy-desktop
if ($LASTEXITCODE -ne 0) { throw 'Desktop Go build failed. Close OpsBuddy before rebuilding.' }
$portableReadme = @'
OpsBuddy 0.17.1 - Windows desktop preview

Double-click OpsBuddy.exe. No Node.js, Go compiler or browser startup is needed.
The UI and Go backend are embedded in this application.
Requires the Microsoft Edge WebView2 Runtime.
Store the app in a writable folder. User data is in .opsbuddy next to the EXE.
Keep that entire folder to preserve projects, models, and generated files.

Native features: file picker, Save As, open with default application,
reveal in Explorer, resize/maximize/minimize, single instance and close cleanup.

This build is a development preview and is not code-signed.
For implemented and planned features, see docs/implementation-status.md.
'@
$portableReadme.Replace('Double-click OpsBuddy.exe.', ('Double-click ' + $BinaryName + '.')) | Set-Content -LiteralPath (Join-Path $releaseRoot 'README.txt') -Encoding UTF8
$archive = Join-Path $workspaceRoot 'release\OpsBuddy-Windows-amd64.zip'
Write-Host ('Desktop EXE: ' + (Join-Path $releaseRoot $BinaryName))
if ($Package) {
    Compress-Archive -LiteralPath (Join-Path $releaseRoot $BinaryName),(Join-Path $releaseRoot 'README.txt') -DestinationPath $archive -Force
    Write-Host ('Portable package: ' + $archive)
}
