@echo off

go build -ldflags "-X 'main.APPLICATION_VERSION=debug'" -o yt-stream-manager-debug.exe .

echo Done!