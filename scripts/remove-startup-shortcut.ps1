Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$startupDir = [Environment]::GetFolderPath("Startup")

function New-Text {
  param(
    [Parameter(Mandatory = $true)]
    [int[]]$CodePoints
  )

  return -join ($CodePoints | ForEach-Object { [char]$_ })
}

$shortcutPath = Join-Path $startupDir ((New-Text @(29983,22270,24037,20316,21488,45,24320,26426,21551,21160)) + ".lnk")

if (Test-Path $shortcutPath) {
  Remove-Item $shortcutPath -Force
  Write-Host "[startup] removed:"
  Write-Host "  $shortcutPath"
  exit 0
}

Write-Host "[startup] shortcut not found:"
Write-Host "  $shortcutPath"
