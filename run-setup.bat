@echo off
REM Runner for setup.exe on the VM.
REM Place next to setup.exe and double-click. Auto-elevates, keeps window open.

cd /d "%~dp0"

REM Relaunch elevated if not already.
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Requesting administrator rights...
    powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

if not exist setup.exe (
    echo setup.exe not found next to this script.
    pause
    exit /b 1
)

echo.
echo === Running setup.exe ===
echo.
setup.exe %*
set EXITCODE=%ERRORLEVEL%
echo.
echo === setup.exe exited with code %EXITCODE% ===
pause
exit /b %EXITCODE%