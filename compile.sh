#! /bin/sh

#export GOOS=windows
#export GOARCH=amd64
#export CGO_ENABLED=1
#export CC=x86_64-w64-mingw32-gcc
#go build -o BusinessAppBSOD.exe main.go

# 32 bit Win
#export GOARCH=386
#export CC=i686-w64-mingw32-gcc

rm launcher.exe launcher-amd launcher-arm launcher-linuxamd launcher-linuxarm

GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -trimpath -o launcher.exe

GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -trimpath -o launcher-amd
GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s" -trimpath -o launcher-arm
cp launcher-arm launcher

GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -trimpath -o launcher-linuxamd
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -trimpath -o launcher-linuxarm

# set CGO_ENABLED=1
# go build -ldflags="-H=windowsgui"


#go build -buildmode=c-shared -o BusinessApp.dll busappdll.go
#go build -o BusinessApp.exe main.go
