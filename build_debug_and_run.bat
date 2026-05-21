@echo off

call build_debug.bat

if %ERRORLEVEL% NEQ 0 (
	rem Nothing...
) else (
	autoytdlpmanager-debug.exe
)
