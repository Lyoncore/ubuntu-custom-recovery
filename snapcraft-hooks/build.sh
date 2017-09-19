#!/bin/sh
# This is the hook for snapcraft build of recovery part

GOPATH=$SNAPCRAFT_PART_INSTALL/../go GOARCH=arm GOARM=7 CGO_ENABLED=1 CC=arm-linux-gnueabihf-gcc go run build.go build
