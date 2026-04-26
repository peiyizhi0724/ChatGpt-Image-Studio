param(
  [ValidateSet("start", "stop", "restart", "status")]
  [string]$Action = "start"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$webDir = Join-Path $repoRoot "web"
$backendDir = Join-Path $repoRoot "backend"
$runtimeDir = Join-Path $repoRoot ".runtime"
$pidFile = Join-Path $runtimeDir "studio.pid"
$logOutFile = Join-Path $runtimeDir "studio.out.log"
$logErrFile = Join-Path $runtimeDir "studio.err.log"
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
New-Item -ItemType Directory -Path $runtimeDir, $env:GOMODCACHE, $env:GOCACHE -Force | Out-Null

function Get-ServerPort {
  $configPath = Join-Path $repoRoot "backend\data\config.toml"
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

function Stop-PidIfRunning {
  param([string]$PidValue)
  if ([string]::IsNullOrWhiteSpace($PidValue)) {
    return
  }
  $pidInt = 0
  if (-not [int]::TryParse($PidValue.Trim(), [ref]$pidInt)) {
    return
  }
  $proc = Get-Process -Id $pidInt -ErrorAction SilentlyContinue
  if ($proc) {
    Stop-Process -Id $pidInt -Force -ErrorAction SilentlyContinue
  }
}

function Stop-ByPort {
  param([int]$Port)
  $output = netstat -ano | Select-String -Pattern "[:.]$Port\s+.*LISTENING\s+(\d+)$"
  foreach ($line in $output) {
    if ($line.Matches.Count -gt 0) {
      $targetPid = [int]$line.Matches[0].Groups[1].Value
      if ($targetPid -gt 0) {
        Stop-Process -Id $targetPid -Force -ErrorAction SilentlyContinue
      }
    }
  }
}

function Stop-RepoHelperProcesses {
  $escapedRoot = [Regex]::Escape($repoRoot)
  $procs = @()
  try {
    $procs = Get-CimInstance Win32_Process -ErrorAction Stop | Where-Object {
      $cmd = [string]$_.CommandLine
      if ([string]::IsNullOrWhiteSpace($cmd)) {
        return $false
      }
      ($cmd -match $escapedRoot) -and ($cmd -match '(vite|node|npm|go run|chatgpt-image-studio)')
    }
  } catch {
    Write-Host "[studioctl] skip helper process scan: $($_.Exception.Message)"
    return
  }
  foreach ($proc in $procs) {
    Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
  }
}

function Build-Frontend {
  Write-Host "[studioctl] building frontend static assets..."
  Push-Location $webDir
  try {
    if (-not (Test-Path (Join-Path $webDir "node_modules\.bin\vite")) -or -not (Test-Path (Join-Path $webDir "node_modules\.bin\esbuild"))) {
      npm ci --ignore-scripts
    }
    npm run build
  } finally {
    Pop-Location
  }
}

function Start-Studio {
  $port = Get-ServerPort
  if (Test-Path $pidFile) {
    Stop-PidIfRunning (Get-Content $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
    Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
  }

  Stop-ByPort -Port $port
  Stop-RepoHelperProcesses
  Build-Frontend

  Write-Host "[studioctl] starting backend on port $port..."
  $process = Start-Process -FilePath $goExe -ArgumentList @("run", ".") -WorkingDirectory $backendDir -RedirectStandardOutput $logOutFile -RedirectStandardError $logErrFile -PassThru -WindowStyle Hidden
  Set-Content -Path $pidFile -Value $process.Id
  Start-Sleep -Milliseconds 800
  if (-not (Get-Process -Id $process.Id -ErrorAction SilentlyContinue)) {
    throw "[studioctl] failed to start backend. check logs: $logOutFile / $logErrFile"
  }
  Write-Host "[studioctl] started. pid=$($process.Id), logs=$logOutFile / $logErrFile"
}

function Stop-Studio {
  $port = Get-ServerPort
  if (Test-Path $pidFile) {
    Stop-PidIfRunning (Get-Content $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
    Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
  }
  Stop-ByPort -Port $port
  Stop-RepoHelperProcesses
  Write-Host "[studioctl] stopped."
}

function Show-Status {
  if (Test-Path $pidFile) {
    $pidValue = Get-Content $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1
    $pidInt = 0
    if ([int]::TryParse([string]$pidValue, [ref]$pidInt) -and (Get-Process -Id $pidInt -ErrorAction SilentlyContinue)) {
      Write-Host "[studioctl] running. pid=$pidInt, logs=$logOutFile / $logErrFile"
      return
    }
  }
  Write-Host "[studioctl] not running."
}

switch ($Action) {
  "start" { Start-Studio }
  "stop" { Stop-Studio }
  "restart" {
    Stop-Studio
    Start-Studio
  }
  "status" { Show-Status }
}
