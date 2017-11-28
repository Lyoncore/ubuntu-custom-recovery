package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"
)

const EFIBOOTMGR = "efibootmgr"
const LOADER = "\\EFI\\BOOT\\BOOTX64.EFI"

func findSysBootEfi(parts *Partitions) (string, error) {
	// find out the boot efi name
	if _, err := os.Stat(SYSBOOT_MNT_DIR); err != nil {
		err := os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
		rplib.Checkerr(err)
	}
	if err := syscall.Mount(fmtPartPath(parts.TargetDevPath, parts.Sysboot_nr), SYSBOOT_MNT_DIR, "vfat", 0, ""); err != nil {
		return "", err
	}
	defer syscall.Unmount(SYSBOOT_MNT_DIR, 0)

	var efiFile string
	err := filepath.Walk(SYSBOOT_MNT_DIR, func(path string, f os.FileInfo, er error) error {
		if !f.IsDir() {
			if r, err := regexp.MatchString("(?i)bootx64.efi", f.Name()); err == nil && r {
				efiFile = strings.Trim(path, SYSBOOT_MNT_DIR)
				return io.EOF
			} else if r, err := regexp.MatchString("(?i)shimx64.efi", f.Name()); err == nil && r {
				efiFile = strings.Trim(path, SYSBOOT_MNT_DIR)
				return io.EOF
			}
		}
		return nil
	})

	if err == io.EOF {
		err = nil
	}

	return efiFile, err
}

func RestoreBootEntries(parts *Partitions, recoveryType string, os_entry string) error {
	// Detect uefi entry needs to be rebuilt if corructed (only when facotry restore)
	if rplib.FACTORY_RESTORE == recoveryType {
		log.Println("[Restoring efi boot entries]")
		recov_entry := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
		os_boot_entry := rplib.GetBootEntries(os_entry)
		if len(recov_entry) < 1 || len(os_boot_entry) < 1 {
			//remove old uefi entry
			entries := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
			for _, entry := range entries {
				rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
			}
			entries = rplib.GetBootEntries(os_entry)
			for _, entry := range entries {
				rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
			}
			// add new uefi entry
			log.Println("[add new uefi entry]")
			rplib.CreateBootEntry(parts.TargetDevPath, parts.Recovery_nr, LOADER, rplib.BOOT_ENTRY_RECOVERY)

			log.Println("[add system-boot entry]")
			if loader, err := findSysBootEfi(parts); err == nil {
				rplib.CreateBootEntry(parts.TargetDevPath, parts.Sysboot_nr, loader, os_entry)
			} else {
				return fmt.Errorf("System boot entry missing, reboot to recovery")
			}

			return fmt.Errorf("Boot entries corrupted has been fixed, reboot system")
		}
	}

	return nil
}

func UpdateBootEntries(parts *Partitions, os_entry string) {
	log.Println("[remove past uefi entry]")
	entries := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
	for _, entry := range entries {
		rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
	}
	entries = rplib.GetBootEntries(os_entry)
	for _, entry := range entries {
		rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
	}

	log.Println("[add new uefi entry]")
	rplib.CreateBootEntry(parts.TargetDevPath, parts.Recovery_nr, LOADER, rplib.BOOT_ENTRY_RECOVERY)
	rplib.CreateBootEntry(parts.TargetDevPath, parts.Sysboot_nr, LOADER, os_entry)
}
