@echo off

set CGO_ENABLED=1
go build -ldflags "-X 'main.APPLICATION_VERSION_TYPE=debug'" -o autoytdlpmanager-debug.exe .

if %ERRORLEVEL% NEQ 0 (
	pause
)
