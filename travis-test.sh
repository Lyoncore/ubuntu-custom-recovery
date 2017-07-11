#!/bin/bash -x
export PATH=/sbin/:$PATH
sudo chown travis:travis /
cd src/
vido --pass-env GOPATH GOROOT GOTOOLDIR GO15VENDOREXPERIMENT --kernel=tests/linux-uml -- go test
sudo chown root:root /
