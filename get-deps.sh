#!/bin/sh

set -eu

if [ -z "$(which godeps)" ];then
	echo Installing godeps
	go get launchpad.net/godeps
fi
export PATH=$PATH:$GOPATH/bin

echo Obtaining dependencies
godeps -t -u dependencies.tsv
