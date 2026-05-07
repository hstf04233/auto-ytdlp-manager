@echo off

go build -ldflags="-s -w" -gcflags="-N -l" .

echo Done!