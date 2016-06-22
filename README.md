# config for generic-amd64

## build recovery binary
``` bash
git clone https://github.com/Lyoncore/generic-amd64-config.git
cd generic-amd64-config/
go get launchpad.net/godeps
godeps -t -u dependencies.tsv
go run build.go build
```
