package main

import (
	"fmt"
	"log"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"
)

const EFIBOOTMGR = "efibootmgr"
const LOADER = "\\EFI\\BOOT\\BOOTX64.EFI"

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
			/*
				//FIXME, find out the EFI
				// find out the boot efi name
				if _, err = os.Stat(SYSBOOT_MNT_DIR); err != nil {
					err := os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
					rplib.Checkerr(err)
				}
				if err := syscall.Mount(fmtPartPath(parts.TargetDevPath, parts.Sysboot_nr), SYSBOOT_MNT_DIR, "vfat", 0, ""); err != nil {
					return err
				}
				if _, err = os.Stat(filepath.Join(SYSBOOT_MNT_DIR, "EFI/boot/
			*/
			// add new uefi entry
			log.Println("[add new uefi entry]")
			rplib.CreateBootEntry(parts.TargetDevPath, parts.Recovery_nr, LOADER, rplib.BOOT_ENTRY_RECOVERY)

			log.Println("[add system-boot entry]")
			rplib.CreateBootEntry(parts.TargetDevPath, parts.Sysboot_nr, LOADER, os_entry)

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
