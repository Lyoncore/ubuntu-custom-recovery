// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2017 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"
)

var version string
var commit string
var commitstamp string
var build_date string

// NOTE: this is hardcoded in `devmode-firstboot.sh`; keep in sync
const (
	DISABLE_CLOUD_OPTION = ""
	ASSERTION_DIR        = "/writable/recovery/"
	ASSERTION_BACKUP_DIR = "/tmp/assert_backup/"
	CONFIG_YAML          = "/recovery/config.yaml"
	CONFIG_GADGET_YAML   = "/recovery/gadget.yaml"
	WRITABLE_MNT_DIR     = "/tmp/writableMnt/"
	SYSBOOT_MNT_DIR      = "/tmp/system-boot/"
	RECO_TAR_MNT_DIR     = "/tmp/recoMnt/"
	RECO_FACTORY_DIR     = "/recovery/factory/"
	RECO_ROOT_DIR        = "/run/initramfs/recovery/recovery/"
	SYSBOOT_TARBALL      = RECO_FACTORY_DIR + "system-boot.tar.xz"
	WRITABLE_TARBALL     = RECO_FACTORY_DIR + "writable.tar.xz"
	ROOTFS_SQUASHFS      = RECO_FACTORY_DIR + "rootfs.squashfs"
	LOG_PATH             = WRITABLE_MNT_DIR + "system-data/var/log/recovery/log.txt"

	SYSTEM_DATA_PATH         = WRITABLE_MNT_DIR + "system-data/"
	SNAPS_SRC_PATH           = RECO_FACTORY_DIR + "snaps/"
	DEV_SNAPS_SRC_PATH       = RECO_FACTORY_DIR + "snaps-devmode/"
	ASSERT_PRE_SRC_PATH      = RECO_FACTORY_DIR + "assertions-preinstall/"
	SNAPS_DST_PATH           = SYSTEM_DATA_PATH + "var/lib/snapd/seed/snaps/"
	ASSERT_DST_PATH          = SYSTEM_DATA_PATH + "var/lib/snapd/seed/assertions/"
	SYSTEMD_SYSTEM_DIR       = "/lib/systemd/system/"
	FIRSTBOOT_SREVICE_SCRIPT = "/var/lib/devmode-firstboot/conf.sh"

	SYSBOOT_UBOOT_ENV  = SYSBOOT_MNT_DIR + "uboot.env"
	SYSBOOT_GRUB_ENV   = SYSBOOT_MNT_DIR + "efi/ubuntu/grubenv"
	SYSBOOT_GRUB_CFG   = SYSBOOT_MNT_DIR + "efi/ubuntu/grub.cfg"
	RECO_PART_GRUB_ENV = RECO_ROOT_DIR + "EFI/ubuntu/grubenv"
	RECO_PART_GRUB_CFG = RECO_ROOT_DIR + "EFI/ubuntu/grub.cfg"
	BACKUP_SNAP_PATH   = "/backup_snaps/"

	WRITABLE_INCLUDES_SQUASHFS = "/recovery/writable-includes.squashfs"

	// Ubuntu classic specific
	WRITABLE_ETC_FSTAB      = WRITABLE_MNT_DIR + "etc/fstab"
	WRITABLE_GRUB_40_CUSTOM = WRITABLE_MNT_DIR + "etc/grub.d/40_custom"
)

var configs rplib.ConfigRecovery
var gadgetInfo rplib.GadgetInfo
var RecoveryType string
var RecoveryLabel string
var RecoveryOS string

func parseConfigs(configFilePath string) {
	var configPath string
	if "" == configFilePath {
		configPath = CONFIG_YAML
	} else {
		configPath = configFilePath
	}

	if "" == version {
		version = Version
	}

	commitstampInt64, _ := strconv.ParseInt(commitstamp, 10, 64)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// Load config.yaml
	err := configs.Load(configPath)
	rplib.Checkerr(err)
	log.Println(configs)
}

func getSysbootSizeFromYaml(gadgetPath string) (int, error) {
	// Load config.yaml
	err := gadgetInfo.Load(gadgetPath)
	if err != nil {
		return -1, err
	}

	return gadgetInfo.GetVolumeSizebyLabel(SysbootLabel)
}

// easier for function mocking
var getPartitions = GetPartitions
var restoreParts = RestoreParts
var syscallMount = syscall.Mount

func getBootEntryName(recoveryos string) string {
	switch RecoveryOS {
	case rplib.RECOVERY_OS_UBUNTU_CORE:
		return rplib.BOOT_ENTRY_SNAPPY
	case rplib.RECOVERY_OS_UBUNTU_CLASSIC:
		return rplib.BOOT_ENTRY_UBUNTU_CLASSIC
	}
	return rplib.BOOT_ENTRY_SNAPPY
}

func preparePartitions(parts *Partitions) {
	// TODO: verify the image
	// If this is user triggered factory restore (first time is in factory and should happen automatically), ask user for confirm.
	if rplib.FACTORY_RESTORE == RecoveryType {
		if ConfirmRecovry(nil) == false {
			os.Exit(0x55) //ERESTART
		}

		//backup assertions
		BackupAssertions(parts)
	}

	// rebuild the partitions
	log.Println("[rebuild the partitions]")
	restoreParts(parts, configs.Configs.Bootloader, configs.Configs.PartitionType)

	//Mount writable for logger and restore data
	if _, err := os.Stat(WRITABLE_MNT_DIR); err != nil {
		err := os.MkdirAll(WRITABLE_MNT_DIR, 0755)
		rplib.Checkerr(err)
	}
	err := syscallMount(fmtPartPath(parts.TargetDevPath, parts.Writable_nr), WRITABLE_MNT_DIR, "ext4", 0, "")
	rplib.Checkerr(err)

	//Mount system-boot for logger and restore data
	if _, err = os.Stat(SYSBOOT_MNT_DIR); err != nil {
		err := os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
		rplib.Checkerr(err)
	}
	err = syscallMount(fmtPartPath(parts.TargetDevPath, parts.Sysboot_nr), SYSBOOT_MNT_DIR, "vfat", 0, "")
	rplib.Checkerr(err)
}

// easier for function mocking
var enableLogger = EnableLogger
var copySnapsAsserts = CopySnapsAsserts
var addFirstBootService = AddFirstBootService
var restoreAsserions = RestoreAsserions
var updateUbootEnv = UpdateUbootEnv
var updateGrubCfg = UpdateGrubCfg
var updateBootEntries = UpdateBootEntries
var updateFstab = UpdateFstab
var grubInstall = GrubInstall

func recoverProcess(parts *Partitions, recoveryos string) {
	commitstampInt64, _ := strconv.ParseInt(commitstamp, 10, 64)

	// stream log to stdout and writable partition
	err := enableLogger()
	rplib.Checkerr(err)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// Copy snaps
	log.Println("[Add additional snaps/asserts]")
	err = copySnapsAsserts()
	rplib.Checkerr(err)

	if recoveryos == rplib.RECOVERY_OS_UBUNTU_CORE {
		// add firstboot service for ubuntu core
		log.Println("[Add FIRSTBOOT service]")
		err = addFirstBootService(RecoveryType, RecoveryLabel)
		rplib.Checkerr(err)
	} else if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
		log.Println("[Update fstab]")
		err = updateFstab(parts, recoveryos)
		rplib.Checkerr(err)
	}

	switch RecoveryType {
	case rplib.FACTORY_INSTALL:
		log.Println("[EXECUTE FACTORY INSTALL]")

	case rplib.FACTORY_RESTORE:
		log.Println("[User restores system]")
		// restore assertion if ever signed
		restoreAsserions()
	}

	if configs.Configs.Bootloader == "u-boot" {
		// update uboot env
		log.Println("[Update uboot env]")
		err = updateUbootEnv(RecoveryLabel)
		rplib.Checkerr(err)
	} else if configs.Configs.Bootloader == "grub" {
		// update uboot env
		log.Println("[Update grub cfg/env]")

		var grub_cfg string
		if recoveryos == rplib.RECOVERY_OS_UBUNTU_CORE {
			grub_cfg = SYSBOOT_GRUB_CFG
		} else if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
			grub_cfg = WRITABLE_GRUB_40_CUSTOM
		}
		// mount as writable before editing
		rplib.Shellexec("mount", "-o", "rw,remount", RECO_ROOT_DIR)
		err = updateGrubCfg(RecoveryLabel, grub_cfg, RECO_PART_GRUB_ENV, recoveryos)
		rplib.Shellexec("mount", "-o", "ro,remount", RECO_ROOT_DIR)
		rplib.Checkerr(err)

		// update efi Boot Entries
		log.Println("[Update boot entries]")
		if recoveryos == rplib.RECOVERY_OS_UBUNTU_CORE {
			updateBootEntries(parts, getBootEntryName(RecoveryOS))
		} else if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
			// grub install also updates the boot entries
			grubInstall(WRITABLE_MNT_DIR, SYSBOOT_MNT_DIR, recoveryos, true)
		}
	}
}

var syscallUnMount = syscall.Unmount

func cleanupPartitions() {
	syscallUnMount(WRITABLE_MNT_DIR, 0)
	syscallUnMount(SYSBOOT_MNT_DIR, 0)
}

func main() {
	flag.Parse()
	if len(flag.Args()) != 3 {
		log.Panicf(fmt.Sprintf("Need two arguments. [RECOVERY_TYPE] [RECOVERY_LABEL] [RECOVERY_OS]. Current arguments: %v", flag.Args()))
	}
	// TODO: use enum to represent RECOVERY_TYPE
	RecoveryType, RecoveryLabel, RecoveryOS = flag.Arg(0), flag.Arg(1), flag.Arg(2)
	log.Printf("RECOVERY_TYPE: %s", RecoveryType)
	log.Printf("RECOVERY_LABEL: %s", RecoveryLabel)
	log.Printf("RECOVERY_OS: %s", RecoveryOS)

	// Find boot device, all other partiitons info
	parts, err := getPartitions(RecoveryLabel, RecoveryType)
	if err != nil {
		log.Panicf("Boot device not found, error: %s\n", err)
	}

	parseConfigs(CONFIG_YAML)
	// Check boot entries if corrupted and in recovery mode.
	// Currently only support amd64
	if configs.Configs.Arch == "amd64" {
		if err := RestoreBootEntries(parts, RecoveryType, getBootEntryName(RecoveryOS)); err != nil {
			// When error return which means the boot entries fixed
			log.Println(err)
			os.Exit(0x55) //ERESTART
		}
	}

	// Headless_installer just copy the recovery partition
	if RecoveryType == rplib.HEADLESS_INSTALLER {
		err := CopyRecoveryPart(parts)
		if err != nil {
			os.Exit(-1)
		}
		os.Exit(0)
	}

	sizeMB, err := getSysbootSizeFromYaml(CONFIG_GADGET_YAML)
	if err == nil {
		SetPartitionStartEnd(parts, SysbootLabel, sizeMB, configs.Configs.Bootloader)
	}
	preparePartitions(parts)
	recoverProcess(parts, RecoveryOS)
	cleanupPartitions()
}
