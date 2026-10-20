param([string]$ModelId = '')
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path $PSScriptRoot -Parent
Set-Location $workspaceRoot
$env:GOPATH = Join-Path $workspaceRoot '.cache\go'
$env:GOMODCACHE = Join-Path $workspaceRoot '.cache\gomod'
$env:GOCACHE = Join-Path $workspaceRoot '.cache\gobuild'
$env:TMP = Join-Path $workspaceRoot '.cache\testtmp'
$env:TEMP = $env:TMP
$demoArguments = @('run', './tools/agent-demo', '-out', 'demos/multi-agent-todo', '-execute')
if ($ModelId) { $demoArguments += @('-model', $ModelId) }
& go @demoArguments
if ($LASTEXITCODE -ne 0) { throw 'Demo did not pass; inspect its isolated run records.' }
