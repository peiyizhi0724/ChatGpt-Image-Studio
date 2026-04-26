Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$webDir = Join-Path $repoRoot "web"
$backendDir = Join-Path $repoRoot "backend"
$goExe = Join-Path $repoRoot ".tools\go1.26.2\go\bin\go.exe"

if (-not (Test-Path $goExe)) {
  $goExe = "go"
}

$env:GOMODCACHE = Join-Path $repoRoot ".gomodcache"
$env:GOCACHE = Join-Path $repoRoot ".gocache"
if (-not $env:GOPROXY) {
  $env:GOPROXY = "https://goproxy.cn,direct"
}
if (-not $env:GOSUMDB) {
  $env:GOSUMDB = "off"
}
New-Item -ItemType Directory -Force -Path $env:GOMODCACHE, $env:GOCACHE | Out-Null

function Assert-LastExitCode {
  param(
    [string]$CommandName
  )

  $exitCode = if (Get-Variable -Name LASTEXITCODE -ErrorAction SilentlyContinue) {
    $LASTEXITCODE
  } else {
    0
  }

  if ($exitCode -ne 0) {
    throw "$CommandName failed with exit code $exitCode"
  }
}

Write-Host "[1/3] Building frontend static assets..."
Push-Location $webDir
if (-not (Test-Path (Join-Path $webDir "node_modules\\.bin\\vite")) -or -not (Test-Path (Join-Path $webDir "node_modules\\.bin\\esbuild"))) {
  npm ci --ignore-scripts
}
Assert-LastExitCode "npm ci"
npm run build
Assert-LastExitCode "npm run build"
Pop-Location

Write-Host "[2/3] Starting backend on configured port..."
Push-Location $backendDir
try {
  & $goExe run .
  Assert-LastExitCode "go run ."
} finally {
  Pop-Location
}
