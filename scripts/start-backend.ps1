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

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot "backend"

Push-Location $backendDir
try {
  $env:SERVER_HOST = "0.0.0.0"
  $env:SERVER_PORT = "$Port"
  Write-Host "Starting backend on http://127.0.0.1:$Port"
  go run .
  Assert-LastExitCode "go run ."
} finally {
  Pop-Location
}
