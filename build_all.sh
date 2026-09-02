#!/bin/bash

CGO_ENABLED=1

if command -v windres >/dev/null; then
    windres app.rc -O coff -o app.syso
else
    echo "windres not installed. Windows binaries will build without icons!" >&2
fi

# Windows
GOOS=windows
# amd64
GOARCH=amd64
go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager.exe"
# arm64
GOARCH=arm64
go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager_arm64.exe"

# Linux
GOOS=windows
GOARCH=amd64
go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager"
# arm64
GOARCH=arm64
go build -ldflags="-s -w" -gcflags="-N -l" -o "./builds/autoytdlpmanager_arm64"