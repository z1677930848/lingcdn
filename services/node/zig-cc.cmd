@echo off
setlocal enabledelayedexpansion

set FILTERED_ARGS=
for %%A in (%*) do (
    set "arg=%%~A"
    echo !arg! | findstr /B /C:"--target=" >nul 2>&1
    if errorlevel 1 (
        set "FILTERED_ARGS=!FILTERED_ARGS! %%A"
    )
)

zig cc -target x86_64-linux-gnu %FILTERED_ARGS%
