package rplib

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

func Shellexec(name string, args ...string) {
	log.Printf(name, args)
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	Checkerr(err)
}

func Shellexecoutput(name string, args ...string) string {
	log.Printf(name, args)
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	Checkerr(err)

	return strings.TrimSpace(string(out[:]))
}

func Shellcmd(command string) {
	cmd := exec.Command("sh", "-c", command)
	log.Printf(strings.Join(cmd.Args, " "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	Checkerr(err)
}

func Shellcmdoutput(command string) string {
	cmd := exec.Command("sh", "-c", command)
	log.Printf(strings.Join(cmd.Args, " "))
	out, err := cmd.Output()
	Checkerr(err)

	return strings.TrimSpace(string(out[:]))
}
