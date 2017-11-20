package rplib

import (
	"log"
	"path/filepath"
	"regexp"
)

var SnapNameRegex = regexp.MustCompile("^([^_]*)_")

func FindSnapName(path string) (name string) {
	snapNameArr := SnapNameRegex.FindAllStringSubmatch(filepath.Base(path), -1)
	log.Println("snapNameArr", snapNameArr)
	if nil == snapNameArr {
		return ""
	}

	return snapNameArr[0][1]
}
