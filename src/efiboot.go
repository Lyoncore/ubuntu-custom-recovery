package main

import (
	"fmt"
	"log"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"
)

const EFIBOOTMGR = "efibootmgr"
const LOADER = "\\EFI\\BOOT\\BOOTX64.EFI"

func RestoreBootEntries(parts *Partitions, recoveryType string) error {
	// Detect uefi entry needs to be rebuilt if corructed (only when facotry restore)
	if rplib.FACTORY_RESTORE == recoveryType {
		log.Println("[Restoring efi boot entries]")
		recov_entry := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
		snappy_entry := rplib.GetBootEntries(rplib.BOOT_ENTRY_SNAPPY)
		if len(recov_entry) < 1 || len(snappy_entry) < 1 {
			//remove old uefi entry
			entries := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
			for _, entry := range entries {
				rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
			}
			entries = rplib.GetBootEntries(rplib.BOOT_ENTRY_SNAPPY)
			for _, entry := range entries {
				rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
			}

			// add new uefi entry
			log.Println("[add new uefi entry]")
			rplib.CreateBootEntry(parts.TargetDevPath, parts.Recovery_nr, LOADER, rplib.BOOT_ENTRY_RECOVERY)

			log.Println("[add system-boot entry]")
			rplib.CreateBootEntry(parts.TargetDevPath, parts.Sysboot_nr, LOADER, rplib.BOOT_ENTRY_SNAPPY)

			return fmt.Errorf("Boot entries corrupted has been fixed, reboot system")
		}
	}

	return nil
}

func UpdateBootEntries(parts *Partitions) {
	log.Println("[remove past uefi entry]")
	entries := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
	for _, entry := range entries {
		rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
	}
	entries = rplib.GetBootEntries(rplib.BOOT_ENTRY_SNAPPY)
	for _, entry := range entries {
		rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
	}

	log.Println("[add new uefi entry]")
	rplib.CreateBootEntry(parts.TargetDevPath, parts.Recovery_nr, LOADER, rplib.BOOT_ENTRY_RECOVERY)
	rplib.CreateBootEntry(parts.TargetDevPath, parts.Sysboot_nr, LOADER, rplib.BOOT_ENTRY_SNAPPY)
}
