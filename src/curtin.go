package main

import (
	"log"
	"os"
	"syscall"
)

func envForUbuntuClassicCurtin() error {
	const CURTIN_RECO_ROOT_DIR = "/cdrom"
	if _, err := os.Stat(RECO_ROOT_DIR); os.IsNotExist(err) {
		if err = os.Mkdir(RECO_ROOT_DIR, 0755); err != nil {
			log.Println("create dir ", RECO_ROOT_DIR, "failed", err.Error())
			return err
		}
	}

	log.Printf("bind mount the %s to %s", CURTIN_RECO_ROOT_DIR, RECO_ROOT_DIR)
	if err := syscall.Mount(CURTIN_RECO_ROOT_DIR, RECO_ROOT_DIR, "", syscall.MS_BIND, ""); err != nil {
		log.Println("bind mount failed, ", err.Error())
		return err
	}

	return nil
}
