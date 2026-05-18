@echo off

go build -ldflags "-X 'main.APPLICATION_VERSION=debug'" -o autoytdlpmanager-debug.exe .
