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

	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
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
	WRITABLE_MNT_DIR     = "/tmp/writableMnt/"
	SYSBOOT_MNT_DIR      = "/tmp/system-boot/"
	RECO_FACTORY_DIR     = "/recovery/factory/"
	SYSBOOT_TARBALL      = RECO_FACTORY_DIR + "system-boot.tar.xz"
	WRITABLE_TARBALL     = RECO_FACTORY_DIR + "writable.tar.xz"
	LOG_PATH             = WRITABLE_MNT_DIR + "system-data/var/log/recovery/log.txt"

	SYSTEM_DATA_PATH         = WRITABLE_MNT_DIR + "system-data/"
	SNAPS_SRC_PATH           = RECO_FACTORY_DIR + "snaps/"
	DEV_SNAPS_SRC_PATH       = RECO_FACTORY_DIR + "snaps-devmode/"
	ASSERT_PRE_SRC_PATH      = RECO_FACTORY_DIR + "assertions-preinstall/"
	SNAPS_DST_PATH           = SYSTEM_DATA_PATH + "var/lib/snapd/seed/snaps/"
	ASSERT_DST_PATH          = SYSTEM_DATA_PATH + "var/lib/snapd/seed/assertions/"
	SYSTEMD_SYSTEM_DIR       = "/lib/systemd/system/"
	FIRSTBOOT_SREVICE_SCRIPT = "/var/lib/devmode-firstboot/conf.sh"

	UBOOT_ENV              = SYSBOOT_MNT_DIR + "uboot.env"
	RECOVERY_PARTITION_DIR = "/recovery_partition/"
	UBOOT_ENV_SRC          = RECOVERY_PARTITION_DIR + "uboot.env"
)

var configs rplib.ConfigRecovery
var RecoveryType string
var RecoveryLabel string

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

	flag.Parse()
	if len(flag.Args()) != 2 {
		log.Panicf(fmt.Sprintf("Need two arguments. [RECOVERY_TYPE] and [RECOVERY_LABEL]. Current arguments: %v", flag.Args()))
	}
	// TODO: use enum to represent RECOVERY_TYPE
	RecoveryType, RecoveryLabel = flag.Arg(0), flag.Arg(1)
	log.Printf("RECOVERY_TYPE: %s", RecoveryType)
	log.Printf("RECOVERY_LABEL: %s", RecoveryLabel)

	// Load config.yaml
	err := configs.Load(configPath)
	rplib.Checkerr(err)
	log.Println(configs)
}

// easier for function mocking
var getPartitions = GetPartitions
var restoreParts = RestoreParts
var syscallMount = syscall.Mount

func preparePartitions() {
	// Find boot device, all other partiitons info
	parts, err := getPartitions(RecoveryLabel)
	if err != nil {
		log.Panicf("Boot device not found, error: %s\n", err)
	}

	// TODO: verify the image
	// If this is user triggered factory restore (first time is in factory and should happen automatically), ask user for confirm.
	if rplib.FACTORY_RESTORE == RecoveryType {
		if ConfirmRecovry(nil) == false {
			os.Exit(1)
		}

		//backup assertions
		BackupAssertions(parts)
	}

	// rebuild the partitions
	log.Println("[rebuild the partitions]")
	restoreParts(parts, configs.Configs.Bootloader, configs.Configs.PartitionType)

	//Mount writable for logger and restore data
	if _, err = os.Stat(WRITABLE_MNT_DIR); err != nil {
		err := os.MkdirAll(WRITABLE_MNT_DIR, 0755)
		rplib.Checkerr(err)
	}
	err = syscallMount(fmtPartPath(parts.DevPath, parts.Writable_nr), WRITABLE_MNT_DIR, "ext4", 0, "")
	rplib.Checkerr(err)

	//Mount system-boot for logger and restore data
	if _, err = os.Stat(SYSBOOT_MNT_DIR); err != nil {
		err := os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
		rplib.Checkerr(err)
	}
	err = syscallMount(fmtPartPath(parts.DevPath, parts.Sysboot_nr), SYSBOOT_MNT_DIR, "vfat", 0, "")
	rplib.Checkerr(err)
}

// easier for function mocking
var enableLogger = EnableLogger
var copySnapsAsserts = CopySnapsAsserts
var addFirstBootService = AddFirstBootService
var restoreAsserions = RestoreAsserions
var updateUbootEnv = UpdateUbootEnv

func recoverProcess() {
	commitstampInt64, _ := strconv.ParseInt(commitstamp, 10, 64)

	// stream log to stdout and writable partition
	err := enableLogger()
	rplib.Checkerr(err)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// Copy snaps
	log.Println("[Add additional snaps/asserts]")
	err = copySnapsAsserts()
	rplib.Checkerr(err)

	// add firstboot service
	log.Println("[Add FIRSTBOOT service]")
	err = addFirstBootService(RecoveryType, RecoveryLabel)
	rplib.Checkerr(err)

	switch RecoveryType {
	case rplib.FACTORY_INSTALL:
		log.Println("[EXECUTE FACTORY INSTALL]")
		// update uboot env
		log.Println("Update uboot env(ESP/system-boot)")
		//fsck needs ignore error code
		log.Println("[set next recoverytype to factory_restore]")
		err = updateUbootEnv()
		rplib.Checkerr(err)

	case rplib.FACTORY_RESTORE:
		log.Println("[User restores system]")
		// restore assertion if ever signed
		restoreAsserions()
	}

	//Darren works here
}

var syscallUnMount = syscall.Unmount

func cleanupPartitions() {
	syscallUnMount(WRITABLE_MNT_DIR, 0)
	syscallUnMount(SYSBOOT_MNT_DIR, 0)
}

func main() {
	//setup logger
	//logger.SimpleSetup()

	parseConfigs(CONFIG_YAML)
	preparePartitions()
	recoverProcess()
	cleanupPartitions()
}
