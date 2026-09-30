@echo off
setlocal

REM ============================================================
REM  Meeting Launchpad - build script (sanitized / demo repo)
REM  One source tree produces every terminal instance:
REM  terminal no / port / silent mode are injected at compile time.
REM
REM  Usage:
REM    build.bat          normal build
REM    build.bat demo     same result, plus an explicit "demo build"
REM                       marker (-X main.BuildDemo=true)
REM
REM  NOTE: this repository ships SAMPLE room values only.
REM        Both forms above are sanitized - no real site data.
REM
REM  NOTE: keep this file ASCII-only. cmd.exe mis-parses UTF-8
REM  comments and the script will fail.
REM
REM  Output goes to output\ - single-file, double-click runnable.
REM ============================================================

set VER=v2.1
set OUTDIR=output
set EXENAME=MeetingLaunchpad_%VER%-demo
set LDFLAGS_BASE=-s -w
set DEMO=

if /I "%~1"=="demo" (
    set DEMO=-X main.BuildDemo=true
    echo [mode] demo build ^(sanitized values + demo marker^)
) else (
    echo [mode] normal build ^(sanitized values^)
)

if not exist %OUTDIR% mkdir %OUTDIR%

echo.
echo Building %EXENAME%-room1.exe ...
go build -o %OUTDIR%/%EXENAME%-room1.exe -ldflags "%LDFLAGS_BASE% -X main.BuildRoomNo=1 -X main.BuildPort=8080 -X main.BuildSilent=true %DEMO%" .

echo Building %EXENAME%-room2.exe ...
go build -o %OUTDIR%/%EXENAME%-room2.exe -ldflags "%LDFLAGS_BASE% -X main.BuildRoomNo=2 -X main.BuildPort=8080 -X main.BuildSilent=true %DEMO%" .

echo Building %EXENAME%-room3.exe ...
go build -o %OUTDIR%/%EXENAME%-room3.exe -ldflags "%LDFLAGS_BASE% -X main.BuildRoomNo=3 -X main.BuildPort=8080 -X main.BuildSilent=true %DEMO%" .

echo Building %EXENAME%-room4.exe ...
go build -o %OUTDIR%/%EXENAME%-room4.exe -ldflags "%LDFLAGS_BASE% -X main.BuildRoomNo=4 -X main.BuildPort=8080 -X main.BuildSilent=true %DEMO%" .

REM Ship the start/stop switch together with the binaries.
REM The script locates the exe inside its own folder, so a single copy
REM serves every terminal: just drop it next to the exe you want to control.
if exist StartOrStop_Server.ps1 copy /Y StartOrStop_Server.ps1 %OUTDIR%\StartOrStop_%VER%.ps1 >nul

echo.
echo Build completed. Output files in %OUTDIR%\ :
echo   %EXENAME%-room1.exe  (Terminal 1, Port 8080, Silent)
echo   %EXENAME%-room2.exe  (Terminal 2, Port 8080, Silent)
echo   %EXENAME%-room3.exe  (Terminal 3, Port 8080, Silent)
echo   %EXENAME%-room4.exe  (Terminal 4, Port 8080, Silent)
echo.
echo Tip: per-terminal room info and default meeting no. can be overridden by
echo      placing config.json next to the exe. Without it the built-in values
echo      are used and the service still runs normally.
endlocal
