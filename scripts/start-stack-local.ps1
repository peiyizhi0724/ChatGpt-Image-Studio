Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

param(
  [int]$Port = 7000
)

function Assert-LastExitCode {
  param(
    [string]$CommandName
  )

  if ($LASTEXITCODE -ne 0) {
    throw "$CommandName failed with exit code $LASTEXITCODE"
  }
}

function Ensure-NodeModules {
  param(
    [string]$Dir,
    [string]$Label
  )

  Push-Location $Dir
  try {
    if (-not (Test-Path "node_modules")) {
      Write-Host "Installing $Label dependencies..."
      npm ci
      Assert-LastExitCode "npm ci"
    }
  } finally {
    Pop-Location
  }
}

function Start-Watcher {
  param(
    [string]$Dir,
    [string]$Label
  )

  Push-Location $Dir
  try {
    Write-Host "Building $Label once before watch..."
    npm run build
    Assert-LastExitCode "npm run build"
  } finally {
    Pop-Location
  }

  $out = Join-Path $Dir "vite-watch.out.log"
  $err = Join-Path $Dir "vite-watch.err.log"
  if (Test-Path $out) {
    Remove-Item $out -Force
  }
  if (Test-Path $err) {
    Remove-Item $err -Force
  }

  $watcher = Start-Process -FilePath "npm.cmd" -ArgumentList @("run", "build:watch") -WorkingDirectory $Dir -PassThru -RedirectStandardOutput $out -RedirectStandardError $err
  Start-Sleep -Seconds 2
  if ($watcher.HasExited) {
    throw "$Label watcher exited early, see $err"
  }
  return $watcher
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$webDir = Join-Path $repoRoot "web"
$portalDir = Join-Path $repoRoot "portal"
$backendDir = Join-Path $repoRoot "backend"

Ensure-NodeModules -Dir $webDir -Label "admin frontend"
Ensure-NodeModules -Dir $portalDir -Label "portal frontend"

Write-Host "[1/2] Starting frontend asset watchers for local backend mode..."
$webWatcher = Start-Watcher -Dir $webDir -Label "admin frontend"
$portalWatcher = Start-Watcher -Dir $portalDir -Label "portal frontend"

Write-Host "[2/2] Starting backend on http://127.0.0.1:$Port"
Push-Location $backendDir
try {
  $env:SERVER_HOST = "0.0.0.0"
  $env:SERVER_PORT = "$Port"
  go run .
  Assert-LastExitCode "go run ."
} finally {
  if ($webWatcher -and -not $webWatcher.HasExited) {
    Stop-Process -Id $webWatcher.Id -Force
  }
  if ($portalWatcher -and -not $portalWatcher.HasExited) {
    Stop-Process -Id $portalWatcher.Id -Force
  }
  Pop-Location
}
