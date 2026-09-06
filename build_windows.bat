@echo off

set CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -buildvcs=false .

if %ERRORLEVEL% NEQ 0 (
	pause
)

echo Done!