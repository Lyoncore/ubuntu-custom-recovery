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
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Lyoncore/arm-config/src/part"
	recoverydirs "github.com/Lyoncore/ubuntu-recovery-rplib/dirs/recovery"
	"github.com/mvo5/uboot-go/uenv"
	"github.com/snapcore/snapd/logger"

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
	OEM_SNAPS_PATH           = SYSTEM_DATA_PATH + "/var/lib/oem/"
	SYSTEMD_SYSTEM_DIR       = "/lib/systemd/system/"
	FIRSTBOOT_SREVICE_SCRIPT = "/var/lib/devmode-firstboot/conf.sh"

	UBOOT_ENV = SYSBOOT_MNT_DIR + "uboot.env"
)

func mib2Blocks(size int) int {
	s := size * 1024 * 1024 / 512

	if s%4 != 0 {
		panic(fmt.Sprintf("invalid partition size: %d", s))
	}

	return s
}

func fmtPartPath(devPath string, nr int) string {
	if strings.Contains(devPath, "mmcblk") || strings.Contains(devPath, "mapper/") {
		return fmt.Sprintf("%sp%d", devPath, nr)
	} else {
		return fmt.Sprintf("%s%d", devPath, nr)
	}
}

// TODO: bootloader if need to support grub
func RestoreParts(parts *part.Partitions, bootloader string, partType string) error {
	var dev_path string = strings.Replace(parts.DevPath, "mapper/", "", -1)
	if partType == "gpt" {
		rplib.Shellexec("sgdisk", dev_path, "--randomize-guids", "--move-second-header")
	}

	// Keep system-boot partition, and only mkfs
	if parts.Sysboot_nr == -1 {
		// oops, don't known the location of system-boot.
		// In the u-boot, system-boot would be in fron of recovery partition
		// If we lose system-boot, and we cannot know the proper location
		return fmt.Errorf("Oops, We lose system-boot")
	}
	sysboot_path := fmtPartPath(parts.DevPath, parts.Sysboot_nr)
	cmd := exec.Command("mkfs.vfat", "-F", "32", "-n", part.SysbootLabel, sysboot_path)
	cmd.Run()
	err := os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
	if err != nil {
		return err
	}
	err = syscall.Mount(sysboot_path, SYSBOOT_MNT_DIR, "vfat", 0, "")
	if err != nil {
		return err
	}
	defer syscall.Unmount(SYSBOOT_MNT_DIR, 0)
	cmd = exec.Command("tar", "--xattrs", "-xJvpf", SYSBOOT_TARBALL, "-C", SYSBOOT_MNT_DIR)
	cmd.Run()
	cmd = exec.Command("parted", "-ms", dev_path, "set", strconv.Itoa(parts.Sysboot_nr), "boot", "on")
	cmd.Run()

	// Remove partitions after recovery which include writable partition
	// And do mkfs in writable (For ensure the writable is enlarged)
	parts.Writable_start = parts.Recovery_end + 1
	var writable_start string = fmt.Sprintf("%vB", parts.Writable_start)
	parts.Writable_nr = parts.Recovery_nr + 1 //writable is one after recovery
	var writable_nr string = strconv.Itoa(parts.Writable_nr)
	writable_path := fmtPartPath(parts.DevPath, parts.Writable_nr)

	part_nr := parts.Recovery_nr + 1
	for part_nr <= parts.Last_part_nr {
		cmd = exec.Command("parted", "-ms", dev_path, "rm", fmt.Sprintf("%v", part_nr))
		cmd.Run()
		part_nr++
	}

	if partType == "gpt" {
		cmd = exec.Command("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "ext4", writable_start, "-1M", "name", writable_nr, part.WritableLabel)
		cmd.Run()
	} else { //mbr
		cmd = exec.Command("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "fat32", writable_start, "-1M")
		cmd.Run()
	}

	cmd = exec.Command("udevadm", "settle")
	cmd.Run()

	cmd = exec.Command("mkfs.ext4", "-F", "-L", part.WritableLabel, writable_path)
	cmd.Run()
	err = os.MkdirAll(WRITABLE_MNT_DIR, 0755)
	rplib.Checkerr(err)
	err = syscall.Mount(writable_path, WRITABLE_MNT_DIR, "ext4", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount(WRITABLE_MNT_DIR, 0)
	cmd = exec.Command("tar", "--xattrs", "-xJvpf", WRITABLE_TARBALL, "-C", WRITABLE_MNT_DIR)
	cmd.Run()

	return nil
}

func hack_grub_cfg(recovery_type_cfg string, recovery_type_label string, recovery_part_label string, grub_cfg string) {
	// add cloud-init disabled option
	// sed -i "s/^set cmdline="\(.*\)"$/set cmdline="\1 $cloud_init_disabled"/g"
	rplib.Shellexec("sed", "-i", "s/^set cmdline=\"\\(.*\\)\"$/set cmdline=\"\\1 $cloud_init_disabled\"/g", grub_cfg)

	// add recovery grub menuentry
	f, err := os.OpenFile(grub_cfg, os.O_APPEND|os.O_WRONLY, 0600)
	rplib.Checkerr(err)

	text := fmt.Sprintf(`
menuentry "%s" {
        # load recovery system
        echo "[grub.cfg] load %s system"
        search --no-floppy --set --label "%s"
        echo "[grub.cfg] root: ${root}"
        set cmdline="root=LABEL=%s ro init=/lib/systemd/systemd console=ttyS0 console=tty1 panic=-1 -- recoverytype=%s"
        echo "[grub.cfg] loading kernel..."
        loopback loop0 /kernel.snap
        linux (loop0)/vmlinuz $cmdline
        echo "[grub.cfg] loading initrd..."
        initrd /initrd.img
        echo "[grub.cfg] boot..."
        boot
}`, recovery_type_label, recovery_type_cfg, recovery_part_label, recovery_part_label, recovery_type_cfg)
	if _, err = f.WriteString(text); err != nil {
		panic(err)
	}

	f.Close()
}

func updateUbootEnv(parts *part.Partitions) error {
	// mount system-boot to update uboot.env
	_, err := os.Stat(SYSBOOT_MNT_DIR)
	if err != nil {
		err = os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
		if err != nil {
			return err
		}
	}
	err = syscall.Mount(fmtPartPath(parts.DevPath, parts.Sysboot_nr), SYSBOOT_MNT_DIR, "vfat", 0, "")
	if err != nil {
		return err
	}
	defer syscall.Unmount(SYSBOOT_MNT_DIR, 0)

	// update uboot.env
	env, err := uenv.Open(UBOOT_ENV)
	if err != nil {
		return err
	}

	var name, value string
	//update env
	//1. mmcreco=1
	name = "mmcreco"
	value = "1"
	env.Set(name, value)
	err = env.Save()
	if err != nil {
		return err
	}

	//2. mmcpart=2
	name = "mmcpart"
	value = "2"
	env.Set(name, value)
	err = env.Save()
	if err != nil {
		return err
	}

	//3. snappy_boot
	name = "snappy_boot"
	value = "if test \"${snap_mode}\" = \"try\"; then setenv snap_mode \"trying\"; saveenv; if test \"${snap_try_core}\" != \"\"; then setenv snap_core \"${snap_try_core}\"; fi; if test \"${snap_try_kernel}\" != \"\"; then setenv snap_kernel \"${snap_try_kernel}\"; fi; elif test \"${snap_mode}\" = \"trying\"; then setenv snap_mode \"\"; saveenv; elif test \"${snap_mode}\" = \"recovery\"; then setenv loadinitrd \"load mmc ${mmcdev}:${mmcreco} ${initrd_addr} ${initrd_file}; setenv initrd_size ${filesize}\"; setenv loadkernel \"load mmc ${mmcdev}:${mmcreco} ${loadaddr} ${kernel_file}\"; setenv factory_recovery \"run loadfiles; setenv mmcroot \"/dev/disk/by-label/writable ${snappy_cmdline} snap_core=${snap_core} snap_kernel=${snap_kernel} recoverytype=factory_restore\"; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} 0x02000000\"; echo \"RECOVERY\"; run factory_recovery; fi; run loadfiles; setenv mmcroot \"/dev/disk/by-label/writable ${snappy_cmdline} snap_core=${snap_core} snap_kernel=${snap_kernel}\"; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} 0x02000000"
	env.Set(name, value)
	err = env.Save()
	if err != nil {
		return err
	}

	//4. loadbootenv (load uboot.env from system-boot, because snapd always update uboot.env in system-boot while os/kernel snap updated)
	name = "loadbootenv"
	value = "load ${devtype} ${devnum}:${mmcpart} ${loadaddr} ${bootenv}"
	env.Set(name, value)
	err = env.Save()
	if err != nil {
		return err
	}

	//5. bootenv (for system-boot/uboot.env)
	name = "bootenv"
	value = "uboot.env"
	env.Set(name, value)
	err = env.Save()

	return err
}

var configs rplib.ConfigRecovery

func ConfirmRecovry(in *os.File) bool {
	//in is for golang testing input.
	//Get user input, if in is nil
	if in == nil {
		in = os.Stdin
	}
	ioutil.WriteFile("/proc/sys/kernel/printk", []byte("0 0 0 0"), 0644)

	fmt.Println("Factory Restore will delete all user data, are you sure? [y/N] ")
	var input string
	fmt.Fscanf(in, "%s\n", &input)
	ioutil.WriteFile("/proc/sys/kernel/printk", []byte("4 4 1 7"), 0644)

	if "y" != input && "Y" != input {
		return false
	}

	return true
}

func BackupAssertions(parts *part.Partitions) error {
	// back up serial assertion
	err := os.MkdirAll(WRITABLE_MNT_DIR, 0755)
	if err != nil {
		return err
	}
	err = syscall.Mount(fmtPartPath(parts.DevPath, parts.Writable_nr), WRITABLE_MNT_DIR, "ext4", 0, "")
	if err != nil {
		return err
	}
	defer syscall.Unmount(WRITABLE_MNT_DIR, 0)

	// back up assertion if ever signed
	if _, err := os.Stat(filepath.Join(WRITABLE_MNT_DIR, ASSERTION_DIR)); err == nil {
		src := filepath.Join(WRITABLE_MNT_DIR, ASSERTION_DIR)
		err = os.MkdirAll(ASSERTION_BACKUP_DIR, 0755)
		if err != nil {
			return err
		}
		dst := ASSERTION_BACKUP_DIR

		err = rplib.CopyTree(src, dst)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}

	return nil
}

func RestoreAsserions() error {

	if _, err := os.Stat(ASSERTION_BACKUP_DIR); err == nil {
		// mount writable to restore
		log.Println("Restore gpg key and serial")
		return rplib.CopyTree(ASSERTION_BACKUP_DIR, filepath.Join(WRITABLE_MNT_DIR, ASSERTION_DIR))
	}

	return nil
}

func EnableLogger() error {
	if _, err := os.Stat(path.Dir(LOG_PATH)); err != nil {
		err = os.MkdirAll(path.Dir(LOG_PATH), 0755)
		if err != nil {
			return err
		}
	}
	log_writable, err := os.OpenFile(LOG_PATH, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	f := io.MultiWriter(log_writable, os.Stdout)
	log.SetOutput(f)
	return nil
}

func CopySnaps() error {
	if _, err := os.Stat(OEM_SNAPS_PATH); err != nil {
		err = os.MkdirAll(OEM_SNAPS_PATH, 0755)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(SNAPS_SRC_PATH); err == nil {
		err = rplib.CopyTree(SNAPS_SRC_PATH, OEM_SNAPS_PATH)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(DEV_SNAPS_SRC_PATH); err == nil {
		err = rplib.CopyTree(DEV_SNAPS_SRC_PATH, OEM_SNAPS_PATH)
		if err != nil {
			return err
		}
	}

	return nil
}

func AddFirstBootService(RecoveryType, RecoveryLabel string) error {
	// before add firstboot service
	// we need to do what "writable-paths" normally does on
	// boot for etc/systemd/system, i.e. copy all the stuff
	// from the os into the writable partition. normally
	// this is the job of the initrd, however it won't touch
	// the dir if there are files in there already. and a
	// kernel/os install will create auto-mount units in there
	// TODO: this is workaround, better to copy from os.snap

	src := filepath.Join("etc", "systemd", "system")
	dst := filepath.Join(SYSTEM_DATA_PATH, "etc", "systemd", "system")
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	if err := rplib.CopyTree(src, dst); err != nil {
		return err
	}

	// unpack writable_local-include
	cmd := exec.Command("unsquashfs", "-f", "-d", WRITABLE_MNT_DIR, recoverydirs.WritableLocalIncludeSquashfs)
	err := cmd.Run()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(SYSTEM_DATA_PATH, FIRSTBOOT_SREVICE_SCRIPT), []byte(fmt.Sprintf("RECOVERYFSLABEL=\"%s\"\nRECOVERY_TYPE=\"%s\"\n", RecoveryLabel, RecoveryType)), 0644)
	if err != nil {
		return err
	}

	return nil
}

//FIXME: now only support eth0, enx0 interface
func startupNetwork() error {
	interface_list := strings.Split(rplib.Shellcmdoutput("ip -o link show | awk -F': ' '{print $2}'"), "\n")
	//log.Println("interface_list:", interface_list)

	var net = 0
	for ; net < len(interface_list); net++ {
		if strings.Contains(interface_list[net], "eth") == true || strings.Contains(interface_list[net], "enx") == true {
			break
		}
	}
	if net == len(interface_list) {
		return fmt.Errorf("No network interface avalible. Current network interface: %v", interface_list)
	}
	eth := interface_list[net] // select nethernet interface.
	cmd := exec.Command("ip", "link", "set", "dev", eth, "up")
	err := cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("dhclient", "-1", eth)
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func releaseDhcp() error {
	cmd := exec.Command("dhclient", "-x")
	return cmd.Run()
}

func serialVaultService() error {
	vaultServerIP := rplib.Shellcmdoutput("ip route | awk '/default/ { print $3 }'") // assume identity-vault is hosted on the gateway
	log.Println("vaultServerIP:", vaultServerIP)

	if !configs.Recovery.SignSerial {
	}
	// TODO: Start signing serial
	return nil
}

func main() {
	//setup logger
	logger.SimpleSetup()

	if "" == version {
		version = Version
	}

	commitstampInt64, _ := strconv.ParseInt(commitstamp, 10, 64)
	logger.Noticef("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	flag.Parse()
	if len(flag.Args()) != 2 {
		logger.Noticef(fmt.Sprintf("Need two arguments. [RECOVERY_TYPE] and [RECOVERY_LABEL]. Current arguments: %v", flag.Args()))
	}
	// TODO: use enum to represent RECOVERY_TYPE
	var RecoveryType, RecoveryLabel = flag.Arg(0), flag.Arg(1)
	logger.Debugf("RECOVERY_TYPE: ", RecoveryType)
	logger.Debugf("RECOVERY_LABEL: ", RecoveryLabel)

	// Load config.yaml
	err := configs.Load(CONFIG_YAML)
	rplib.Checkerr(err)
	log.Println(configs)

	// Find boot device, all other partiitons info
	parts, err := part.GetPartitions(RecoveryLabel)
	if err != nil {
		logger.Panicf("Boot device not found, error: %s\n", err)
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
	RestoreParts(parts, configs.Configs.Bootloader, configs.Configs.PartitionType)

	//Mount writable for logger and restore data
	if _, err = os.Stat(WRITABLE_MNT_DIR); err != nil {
		err := os.MkdirAll(WRITABLE_MNT_DIR, 0755)
		rplib.Checkerr(err)
	}
	err = syscall.Mount(fmtPartPath(parts.DevPath, parts.Writable_nr), WRITABLE_MNT_DIR, "ext4", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount(WRITABLE_MNT_DIR, 0)

	// stream log to stdout and writable partition
	err = EnableLogger()
	rplib.Checkerr(err)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// Copy snaps
	log.Println("[Add snaps for oem]")
	err = CopySnaps()
	rplib.Checkerr(err)

	// add firstboot service
	log.Println("[Add FIRSTBOOT service]")
	err = AddFirstBootService(RecoveryType, RecoveryLabel)
	rplib.Checkerr(err)

	switch RecoveryType {
	case rplib.FACTORY_INSTALL:
		log.Println("[EXECUTE FACTORY INSTALL]")
		// TODO: read assertion information from gadget snap
		if !configs.Recovery.SignSerial {
			log.Println("[Start serial vault]")
			err = startupNetwork()
			rplib.Checkerr(err)
			err = serialVaultService()
			rplib.Checkerr(err)
			//release dhclient
			err = releaseDhcp()
			rplib.Checkerr(err)
		}
	case rplib.FACTORY_RESTORE:
		log.Println("[User restores system]")
		// restore assertion if ever signed
		RestoreAsserions()
	}

	//Darren works here
	// update uboot env
	log.Println("Update uboot env(ESP/system-boot)")
	//fsck needs ignore error code
	log.Println("[set next recoverytype to factory_restore]")
	err = updateUbootEnv(parts)
	rplib.Checkerr(err)
}
