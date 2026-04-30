Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

param(
  [string]$ApiTarget = "https://mimo.iqei.cn",
  [int]$Port = 5174
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
$portalDir = Join-Path $repoRoot "portal"
$target = $ApiTarget.Trim().TrimEnd("/")

if ([string]::IsNullOrWhiteSpace($target)) {
  throw "ApiTarget cannot be empty."
}

Push-Location $portalDir
try {
  if (-not (Test-Path "node_modules")) {
    Write-Host "[1/2] Installing portal frontend dependencies..."
    npm ci
    Assert-LastExitCode "npm ci"
  }

  Write-Host "[2/2] Starting portal frontend on http://127.0.0.1:$Port"
  Write-Host "Proxy target: $target"
  $env:VITE_API_PROXY_TARGET = $target
  Remove-Item Env:VITE_API_URL -ErrorAction SilentlyContinue
  npm run dev -- --port $Port
  Assert-LastExitCode "npm run dev"
} finally {
  Pop-Location
}
