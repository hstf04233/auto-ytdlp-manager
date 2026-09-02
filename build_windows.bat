@echo off

go build -ldflags="-s -w" .

if %ERRORLEVEL% NEQ 0 (
	pause
)

echo Done!