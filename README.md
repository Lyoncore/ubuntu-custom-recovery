# config for arm

## Prerequisites
- ubuntu-recovery-image: could be install from http://github.com/Lyoncore/ubuntu-recovery-image
- Branch for arm-config: arm


## Build image
``` bash
git clone https://github.com/Lyoncore/ubuntu-recovery.git
cd ubuntu-recovery/
go get launchpad.net/godeps
godeps -t -u dependencies.tsv
```

### For armhf (ex: pi3)

Build recovery.bin
``` bash
GOARCH=arm GOARM=7 CGO_ENABLED=1 CC=arm-linux-gnueabihf-gcc go run build.go build
```
Build base image
``` bash
./cook-image.sh
```

### For arm64
``` bash
GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc go build -o local-includes/recovery/bin/recovery.bin ./src/
```

## Generate image with recovery
``` bash
<Path to>/ubuntu-recovery-image
```

## run tests
``` bash
cd src
go test -check.vv
```
