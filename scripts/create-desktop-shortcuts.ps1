Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$desktopDir = [Environment]::GetFolderPath("Desktop")

function New-Text {
  param(
    [Parameter(Mandatory = $true)]
    [int[]]$CodePoints
  )

  return -join ($CodePoints | ForEach-Object { [char]$_ })
}

if ([string]::IsNullOrWhiteSpace($desktopDir) -or -not (Test-Path $desktopDir)) {
  throw "Desktop folder not found."
}

$shell = New-Object -ComObject WScript.Shell

$shortcuts = @(
  @{
    Name = (New-Text @(29983,22270,24037,20316,21488,45,21551,21160))
    Target = Join-Path $repoRoot "start-studio.cmd"
    Description = (New-Text @(21551,21160,32,67,104,97,116,71,112,116,32,73,109,97,103,101,32,83,116,117,100,105,111,32,24182,33258,21160,25171,24320,25171,32593,39029))
  },
  @{
    Name = (New-Text @(29983,22270,24037,20316,21488,45,20572,27490))
    Target = Join-Path $repoRoot "stop-studio.cmd"
    Description = (New-Text @(20572,27490,32,67,104,97,116,71,112,116,32,73,109,97,103,101,32,83,116,117,100,105,111))
  },
  @{
    Name = (New-Text @(29983,22270,24037,20316,21488,45,29366,24577))
    Target = Join-Path $repoRoot "studio-status.cmd"
    Description = (New-Text @(26597,30475,32,67,104,97,116,71,112,116,32,73,109,97,103,101,32,83,116,117,100,105,111,32,24403,21069,36816,34892,29366,24577))
  },
  @{
    Name = (New-Text @(29983,22270,24037,20316,21488,45,26085,24535))
    Target = Join-Path $repoRoot "open-studio-logs.cmd"
    Description = (New-Text @(25171,24320,32,67,104,97,116,71,112,116,32,73,109,97,103,101,32,83,116,117,100,105,111,32,21551,21160,26085,24535))
  }
)

foreach ($item in $shortcuts) {
  if (-not (Test-Path $item.Target)) {
    throw "Shortcut target not found: $($item.Target)"
  }

  $shortcutPath = Join-Path $desktopDir ($item.Name + ".lnk")
  $shortcut = $shell.CreateShortcut($shortcutPath)
  $shortcut.TargetPath = $item.Target
  $shortcut.WorkingDirectory = $repoRoot
  $shortcut.Description = $item.Description
  $shortcut.IconLocation = "$env:SystemRoot\System32\shell32.dll,220"
  $shortcut.Save()
}

Write-Host "[shortcuts] created on desktop:"
foreach ($item in $shortcuts) {
  Write-Host "  - $($item.Name)"
}
