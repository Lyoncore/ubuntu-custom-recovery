package hooks

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

type hooks interface {
	IsHookExist() bool
	Run() error
}

type RestoreComfirmHooks struct {
	path string
}

func (RCHook *RestoreComfirmHooks) SetPath(path string) {
	RCHook.path = path
}

func (RCHook *RestoreComfirmHooks) IsHookExist() bool {
	if _, err := os.Stat(RCHook.path); os.IsNotExist(err) {
		return false
	} else {
		return true
	}
}

func (RCHook *RestoreComfirmHooks) Run(recoveryMnt string, envValEn bool, envName string, envValue string) error {
	log.Println("Run scripts: " + RCHook.path)
	if RCHook.IsHookExist() {
		cmd := exec.Command("/bin/bash", RCHook.path)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("RECOVERYMNT=%s", recoveryMnt))
		if envValEn {
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", envName, envValue))
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return err
	}
	return fmt.Errorf("Hook not found: %s\n", RCHook.path)
}

var RestoreConfirmPrehook RestoreComfirmHooks
var RestoreConfirmPosthook RestoreComfirmHooks
