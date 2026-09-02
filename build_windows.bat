@echo off

set CGO_ENABLED=1
go build -ldflags="-s -w" .

if %ERRORLEVEL% NEQ 0 (
	pause
)

echo Done!