$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path $PSScriptRoot -Parent
Set-Location $workspaceRoot
$env:GOPATH = Join-Path $workspaceRoot '.cache\go'
$env:GOMODCACHE = Join-Path $workspaceRoot '.cache\gomod'
$env:GOCACHE = Join-Path $workspaceRoot '.cache\gobuild'
$testTemp = Join-Path $workspaceRoot '.cache\testtmp'
New-Item -ItemType Directory -Force -Path $testTemp | Out-Null
$env:TMP = $testTemp
$env:TEMP = $testTemp
& go test ./... -count=1
if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
& go vet ./...
if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
& npm.cmd run build
if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
