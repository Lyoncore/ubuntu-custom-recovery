#!/bin/bash -ex
export PATH=/sbin/:$PATH
sudo chown travis:travis /
./get-deps.sh
cd src/
vido --mem=512M --pass-env GOPATH GOROOT GOTOOLDIR --kernel=tests/linux-uml -- go test
sudo chown root:root /
