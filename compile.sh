#! /bin/sh

#export GOOS=windows
#export GOARCH=amd64
#export CGO_ENABLED=1
#export CC=x86_64-w64-mingw32-gcc
#go build -o BusinessAppBSOD.exe main.go

# 32 bit Win
#export GOARCH=386
#export CC=i686-w64-mingw32-gcc

rm launcher bin/*/launcher*

GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -trimpath -o bin/WinAMD64/launcher.exe
GOOS=windows GOARCH=arm64 go build -ldflags="-w -s" -trimpath -o bin/WinARM64/launcher.exe

GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -trimpath -o bin/MacOSAMD64/launcher
GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s" -trimpath -o bin/MacOSARM64/launcher
cp bin/MacOSARM64/launcher ./launcher

GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -trimpath -o bin/LinuxAMD64/launcher
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -trimpath -o bin/LinuxARM64/launcher


# "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942