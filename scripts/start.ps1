param([switch]$SkipBuild, [switch]$Rebuild)
$ErrorActionPreference = 'Stop'
$workspaceRoot = Split-Path $PSScriptRoot -Parent
Set-Location $workspaceRoot
$desktopExe = Join-Path $workspaceRoot 'release\OpsBuddy\OpsBuddy.exe'
if (-not (Test-Path -LiteralPath $desktopExe) -or ($Rebuild -and -not $SkipBuild)) {
    & (Join-Path $PSScriptRoot 'build-desktop.ps1')
}
if (-not (Test-Path -LiteralPath $desktopExe)) { throw 'Desktop build is missing.' }
# A running older EXE can remain open while a newer desktop build is staged in
# the same directory. All versions use the same existing .opsbuddy data folder.
$latestDesktop = Get-ChildItem -LiteralPath (Split-Path $desktopExe -Parent) -Filter 'OpsBuddy*.exe' -File |
    Where-Object { $_.Name -match '^OpsBuddy(?:-[0-9.]+)?\.exe$' } |
    Sort-Object @{ Expression = { [version]$_.VersionInfo.FileVersion }; Descending = $true }, Name |
    Select-Object -First 1
if ($latestDesktop) { $desktopExe = $latestDesktop.FullName }
Start-Process -FilePath $desktopExe -WorkingDirectory (Split-Path $desktopExe -Parent) -WindowStyle Normal
