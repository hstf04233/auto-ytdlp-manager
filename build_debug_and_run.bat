@echo off

call build_debug.bat

if %ERRORLEVEL% NEQ 0 (
	pause
	exit
)

yt-stream-manager-debug.exe