#!/bin/bash -ex
export PATH=/sbin/:$PATH
sudo chown travis:travis /
./get-deps.sh
cd src/
vido --pass-env GOPATH GOROOT GOTOOLDIR GO15VENDOREXPERIMENT --kernel=tests/linux-uml -- go test
sudo chown root:root /
