@echo off
setlocal
cd /d "%~dp0"
set "PS_EXE=%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe"
if not exist "%PS_EXE%" set "PS_EXE=pwsh"
"%PS_EXE%" -NoProfile -ExecutionPolicy Bypass -File ".\scripts\create-startup-shortcut.ps1"
if errorlevel 1 (
  echo.
  echo Failed to create startup shortcut. Press any key to close this window.
  pause >nul
  exit /b 1
)
echo.
echo Startup shortcut created. Press any key to close this window.
pause >nul
endlocal
