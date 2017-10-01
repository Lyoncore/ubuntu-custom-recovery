#!/bin/bash
# This is the hook for snapcraft build of recovery part

arch=$1

if [ $arch == 'amd64' ]; then
    GOPATH=$SNAPCRAFT_PART_INSTALL/../go go run build.go build
elif [ $arch == "armhf" ];then
    GOPATH=$SNAPCRAFT_PART_INSTALL/../go GOARCH=arm GOARM=7 CGO_ENABLED=1 CC=arm-linux-gnueabihf-gcc go run build.go build
elif [ $arch == "arm64" ];then
    GOPATH=$SNAPCRAFT_PART_INSTALL/../go GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -o local-includes/recovery/bin/recovery.bin ./src/
else
    echo "unknown arch"
    return -1
fi
