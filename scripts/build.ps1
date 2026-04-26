Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$webDir = Join-Path $repoRoot "web"
$backendDir = Join-Path $repoRoot "backend"
$distDir = Join-Path $repoRoot "dist"
$backendStaticDir = Join-Path $backendDir "static"
$packageDir = Join-Path $distDir "package"
$packageDataDir = Join-Path $packageDir "data"
$packageStaticDir = Join-Path $packageDir "static"
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

  if ($LASTEXITCODE -ne 0) {
    throw "$CommandName failed with exit code $LASTEXITCODE"
  }
}

function Resolve-BuildMetadata {
  $version = "dev"
  $commit = "none"
  $buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

  $gitVersion = git -C $repoRoot describe --tags --always --dirty 2>$null
  if ($LASTEXITCODE -eq 0 -and $gitVersion) {
    $version = $gitVersion.Trim()
  }

  $gitCommit = git -C $repoRoot rev-parse --short HEAD 2>$null
  if ($LASTEXITCODE -eq 0 -and $gitCommit) {
    $commit = $gitCommit.Trim()
  }

  return @{
    Version = $version
    Commit = $commit
    BuildTime = $buildTime
  }
}

$buildMeta = Resolve-BuildMetadata
$env:VITE_APP_VERSION = $buildMeta.Version
$ldflags = @(
  "-s",
  "-w",
  "-X chatgpt2api/internal/buildinfo.Version=$($buildMeta.Version)",
  "-X chatgpt2api/internal/buildinfo.Commit=$($buildMeta.Commit)",
  "-X chatgpt2api/internal/buildinfo.BuildTime=$($buildMeta.BuildTime)"
) -join " "

Write-Host "[1/4] Building frontend..."
Push-Location $webDir
if (-not (Test-Path (Join-Path $webDir "node_modules\\.bin\\vite")) -or -not (Test-Path (Join-Path $webDir "node_modules\\.bin\\esbuild"))) {
  npm ci --ignore-scripts
}
Assert-LastExitCode "npm ci"
npm run build
Assert-LastExitCode "npm run build"
Pop-Location

Write-Host "[2/4] Syncing frontend assets..."
if (Test-Path $backendStaticDir) {
  Remove-Item -Recurse -Force $backendStaticDir
}
New-Item -ItemType Directory -Path $backendStaticDir -Force | Out-Null
Copy-Item -Path (Join-Path $webDir "dist\\*") -Destination $backendStaticDir -Recurse -Force

Write-Host "[3/4] Building backend..."
New-Item -ItemType Directory -Path $distDir -Force | Out-Null
Push-Location $backendDir
& $goExe build -ldflags $ldflags -o (Join-Path $distDir "chatgpt-image-studio.exe") .
Assert-LastExitCode "go build"
Pop-Location

Write-Host "[4/4] Preparing local release package..."
if (Test-Path $packageDir) {
  Remove-Item -Recurse -Force $packageDir
}
New-Item -ItemType Directory -Path $packageDataDir -Force | Out-Null
New-Item -ItemType Directory -Path $packageStaticDir -Force | Out-Null
Copy-Item -Path (Join-Path $distDir "chatgpt-image-studio.exe") -Destination (Join-Path $packageDir "chatgpt-image-studio.exe") -Force
Copy-Item -Path (Join-Path $backendDir "internal\\config\\config.defaults.toml") -Destination (Join-Path $packageDataDir "config.example.toml") -Force
Copy-Item -Path (Join-Path $webDir "dist\\*") -Destination $packageStaticDir -Recurse -Force
Copy-Item -Path (Join-Path $repoRoot "packaging\\README.txt") -Destination (Join-Path $packageDir "README.txt") -Force

Write-Host "Build complete: $distDir"
