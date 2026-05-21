@echo off

set CGO_ENABLED=1
go build -ldflags="-s -w" -gcflags="-N -l" .

if %ERRORLEVEL% NEQ 0 (
	pause
)

echo Done!