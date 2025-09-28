.DEFAULT_GOAL := hello

hello:
	echo "Hello KrankyBear!"
	echo "make fmt to format the code"
	echo "make lint to run golint"
	echo "make vet to vet the code"
	echo "make run to run the main.go"
	echo "make build to build for the current system"
	echo "make buildsupported to build for the currently supported systems - MacAMD, MacARM and WinAMD"
	# echo "	make linuxamd64"
	# echo "	make linuxarm64"
	echo "	make macamd64"
	echo "	make macarm64"
	echo "	make winamd64"
	echo "	make winarm64 - not currently working!"
	echo "	make all"
	echo "make clean to remove compiled files from bin/*"
	echo "make doc to generate some docs based on func names"
	echo "  grepped | tee doc.md, on display and in file"
.PHONY:hello

fmt:
	go fmt ./...
.PHONY:fmt

lint:
	~/go/bin/golint ./...
.PHONY:lint

tidy:
	go mod tidy
	go mod vendor
	go mod verify
.PHONY:tidy

vet: 
	fmt
	go vet ...
.PHONY:vet

run:
	go run .
.PHONY:run

# Supported cross compile GOOS and GOARCH https://gist.github.com/asukakenji/f15ba7e588ac42795f421b48b8aede63
build:
	./setver.sh
	go build -ldflags="-w -s" -o KrankyBearClock .
	./setIcon.sh Resources/Images/KrankyBearBeret.png KrankyBearClock
.PHONY:build


ios:
	GOOS=ios CGO_ENABLED=1 go build -ldflags="-w -s" -o bin/ios/
.PHONY:ios

linuxamd64:
 	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-w -s" -o bin/LinuxAMD64/
.PHONY:linuxamd64

linuxarm64:
 	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-w -s" -o bin/LinuxARM64/
.PHONY:linuxarm64

macamd64:
	# GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-w -s" -o bin/MacOSAMD64/
	# ./setIcon.sh KrankyBear.png bin/MacOSAMD64/KrankyBearClock
	./mkicns.sh Resources/Images/KrankyBearBeret.png
	./dmgbuildIntel.sh
.PHONY:macamd64

macarm64:
	# GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-w -s" -o bin/MacOSARM64/
	# ./setIcon.sh KrankyBear.png bin/MacOSAMD64/KrankyBearClock
	./mkicns.sh Resources/Images/KrankyBearBeret.png
	./dmgbuildARM.sh
.PHONY:macarm64

winamd64:
	go-winres make
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC="x86_64-w64-mingw32-gcc" go build -ldflags="-w -s -H windowsgui -r KrankyBearClock.rc" -o bin/WinAMD64/
	echo "Now take this to Windows and run it through innoBuiltIntel.ps1"
.PHONY:winamd64

winarm64:
	go-winres make
	GOOS=windows GOARCH=arm64 CGO_ENABLED=1 CC="aarch64-w64-mingw32-gcc" go build -ldflags="-w -s -H windowsgui -r KrankyBearClock.rc" -o bin/WinARM64/
.PHONY:winarm64


buildall: linuxamd64 linuxarm64 macamd64 macarm64 winamd64 winarm64
.PHONY:buildall

buildsupported supported: macamd64 macarm64 winamd64
.PHONY:buildsupported

dmg: 
	./dmgbuildARM.sh
	./dmgbuildIntel.sh
.PHONY:dmg
	
clean:
	rm bin/*/*
	rm installers/*.dmg
	rm installers/*.exe
.PHONY:clean

doc:
	grep -e "^// .* \.\.\. .*" -e "^.. .* \.\.\." -e "^func .*" *.go | tee doc.md
.PHONY:doc

dmgARM:
	./dmgbuildARM.sh
.PHONY:dmgARM

dmgIntel:
	./dmgbuildIntel.sh
.PHONY:dmgIntel

