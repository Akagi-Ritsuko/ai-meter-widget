@echo off
REM Start Microsoft Edge with CDP debug port 9222 for ai-meter-widget
REM Uses a SEPARATE profile dir to force a fresh process (avoids Edge startup-boost handoff)
REM Usage: double-click or run from command line

set "EDGE_PATH="

REM 1. Find Edge via registry
for /f "tokens=2,*" %%a in ('reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe" /ve 2^>nul') do set "EDGE_PATH=%%b"
if not defined EDGE_PATH for /f "tokens=2,*" %%a in ('reg query "HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe" /ve 2^>nul') do set "EDGE_PATH=%%b"

REM 2. Fallback to common install paths
if not defined EDGE_PATH if exist "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe" set "EDGE_PATH=C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
if not defined EDGE_PATH if exist "C:\Program Files\Microsoft\Edge\Application\msedge.exe" set "EDGE_PATH=C:\Program Files\Microsoft\Edge\Application\msedge.exe"

if not defined EDGE_PATH (
    echo [ERROR] Edge not found. Check installation path.
    pause
    exit /b 1
)

echo Found Edge: %EDGE_PATH%

REM 3. Start Edge with debug port using a SEPARATE profile directory.
REM    A separate profile forces a brand-new process, so --remote-debugging-port
REM    always takes effect (avoids Edge startup-boost handing off to an old process).
set "DEBUG_PROFILE=%LOCALAPPDATA%\ai-meter-widget-edge"
if not exist "%DEBUG_PROFILE%" mkdir "%DEBUG_PROFILE%"

echo Starting Edge (debug port 9222, separate profile)...
start "" "%EDGE_PATH%" --remote-debugging-port=9222 --user-data-dir="%DEBUG_PROFILE%"

echo.
echo Done. Verify: open http://localhost:9222/json/version in browser.
echo IMPORTANT: log into https://console.volcengine.com in the new Edge window once,
echo so the CDP extractor can read the console cookies.
pause