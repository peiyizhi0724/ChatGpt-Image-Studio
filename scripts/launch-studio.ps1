Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$studioCtl = Join-Path $PSScriptRoot "studioctl.ps1"
$configPath = Join-Path $repoRoot "backend\data\config.toml"
$runtimeDir = Join-Path $repoRoot ".runtime"
$logOutFile = Join-Path $runtimeDir "studio.out.log"
$logErrFile = Join-Path $runtimeDir "studio.err.log"

function Get-ServerPort {
  if (-not (Test-Path $configPath)) {
    return 7000
  }

  $lines = Get-Content $configPath
  $inServer = $false
  foreach ($line in $lines) {
    if ($line -match '^\[server\]\s*$') {
      $inServer = $true
      continue
    }
    if ($line -match '^\[') {
      $inServer = $false
    }
    if ($inServer -and $line -match '^\s*port\s*=\s*(\d+)\s*$') {
      return [int]$Matches[1]
    }
  }

  return 7000
}

function Wait-ForStudio {
  param(
    [int]$Port,
    [int]$TimeoutSeconds = 20
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  $url = "http://127.0.0.1:$Port/health"

  while ((Get-Date) -lt $deadline) {
    try {
      $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 2
      if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) {
        return $true
      }
    } catch {
      Start-Sleep -Milliseconds 700
    }
  }

  return $false
}

Write-Host "[launcher] starting ChatGpt Image Studio..."
& $studioCtl start

$port = Get-ServerPort
if (Wait-ForStudio -Port $port) {
  $url = "http://127.0.0.1:$port"
  Write-Host "[launcher] opening $url"
  Start-Process $url | Out-Null
  Write-Host "[launcher] studio is ready."
  exit 0
}

Write-Host "[launcher] studio did not become ready in time."
Write-Host "[launcher] check logs:"
Write-Host "  $logOutFile"
Write-Host "  $logErrFile"
exit 1
