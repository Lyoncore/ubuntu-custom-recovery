package rplib

import (
	"log"
	"os"
	"os/exec"
)

// Panic on error
func Checkerr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func Debugshell() {
	cmd := exec.Command("sh")
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Println(cmd)
		log.Println(err)
	}
}
