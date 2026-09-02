#!/bin/bash

if command -v windres >/dev/null; then
    windres app.rc -O coff -o app.syso
else
    echo "windres not installed. Windows binaries will build without icons!" >&2
	rm -rf "./app.syso"
fi

# Windows
echo Building Windows amd64...
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager.exe"
echo Done

echo Building Windows arm64...
CGO_ENABLED=1 GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager_arm64.exe"
echo Done

# Linux
echo Building Linux amd64...
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager"
echo Done

echo Building Linux arm64...
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager_arm64"
echo Done