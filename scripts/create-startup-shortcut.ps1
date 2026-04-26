Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$startupDir = [Environment]::GetFolderPath("Startup")
$target = Join-Path $repoRoot "start-studio.cmd"

function New-Text {
  param(
    [Parameter(Mandatory = $true)]
    [int[]]$CodePoints
  )

  return -join ($CodePoints | ForEach-Object { [char]$_ })
}

if ([string]::IsNullOrWhiteSpace($startupDir) -or -not (Test-Path $startupDir)) {
  throw "Startup folder not found."
}
if (-not (Test-Path $target)) {
  throw "Startup target not found: $target"
}

$shortcutName = (New-Text @(29983,22270,24037,20316,21488,45,24320,26426,21551,21160)) + ".lnk"
$shortcutPath = Join-Path $startupDir $shortcutName

$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $target
$shortcut.WorkingDirectory = $repoRoot
$shortcut.Description = (New-Text @(24320,26426,21518,26368,23567,21270,21270,21551,21160,32,67,104,97,116,71,112,116,32,73,109,97,103,101,32,83,116,117,100,105,111))
$shortcut.WindowStyle = 7
$shortcut.IconLocation = "$env:SystemRoot\System32\shell32.dll,220"
$shortcut.Save()

Write-Host "[startup] shortcut created:"
Write-Host "  $shortcutPath"
