@echo off
setlocal
cd /d "%~dp0"
if not exist ".runtime" mkdir ".runtime"
if not exist ".runtime\studio.out.log" type nul > ".runtime\studio.out.log"
if not exist ".runtime\studio.err.log" type nul > ".runtime\studio.err.log"
start "" notepad ".runtime\studio.out.log"
start "" notepad ".runtime\studio.err.log"
endlocal
