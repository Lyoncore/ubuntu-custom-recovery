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
	"errors"
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
	DISABLE_CLOUD_OPTION    = ""
	LOG_PATH                = "/writable/system-data/var/log/recovery/log.txt"
	ASSERTION_FOLDER        = "/writable/recovery"
	ASSERTION_BACKUP_FOLDER = "/tmp/assert_backup"
	CONFIG_YAML             = "/recovery/config.yaml"
)

func mib2Blocks(size int) int {
	s := size * 1024 * 1024 / 512

	if s%4 != 0 {
		panic(fmt.Sprintf("invalid partition size: %d", s))
	}

	return s
}

func recreateRecoveryPartition(device string, RecoveryLabel string, recovery_nr int, recovery_end int64) (normal_boot_nr int) {
	last_end := recovery_end
	nr := recovery_nr + 1

	// Read information of recovery partition
	// Keep only recovery partition

	partitions := [...]string{part.SysbootLabel, part.WritableLabel}
	for _, partition := range partitions {
		log.Println("last_end:", last_end)
		log.Println("nr:", nr)

		var end_size string
		var fstype string
		var block string
		// TODO: allocate partition according to gadget.PartitionLayout
		// create the new partition
		if part.WritableLabel == partition {
			end_size = "-1M"
			fstype = "ext4"
		} else if part.SysbootLabel == partition {
			size := 64 * 1024 * 1024 // 64 MB in Bytes
			end_size = strconv.FormatInt(last_end+int64(size), 10) + "B"
			fstype = "fat32"
		}
		log.Println("end_size:", end_size)

		// make partition with optimal alignment
		if configs.Configs.Bootloader == "gpt" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", device, "--", "mkpart", "primary", fstype, fmt.Sprintf("%vB", last_end+1), end_size, "name", fmt.Sprintf("%v", nr), partition)
		} else { //mbr don't support partition name
			rplib.Shellexec("parted", "-a", "optimal", "-ms", device, "--", "mkpart", "primary", fstype, fmt.Sprintf("%vB", last_end+1), end_size)
		}
		_, new_end := rplib.GetPartitionBeginEnd64(device, nr)

		if part.SysbootLabel == partition {
			normal_boot_nr = nr
			log.Println("normal_boot_nr:", normal_boot_nr)
		}

		rplib.Shellexec("udevadm", "settle")
		rplib.Shellexec("parted", "-ms", device, "unit", "B", "print") // debug

		if strings.Contains(device, "mmcblk") == true {
			block = fmt.Sprintf("%sp%d", device, nr) //mmcblk0pX
		} else {
			block = fmt.Sprintf("%s%d", device, nr)
		}
		log.Println("block:", block)
		// create filesystem and dump snap system
		switch partition {
		case part.WritableLabel:
			rplib.Shellexec("mkfs.ext4", "-F", "-L", part.WritableLabel, block)
			err := os.MkdirAll("/tmp/writable/", 0755)
			rplib.Checkerr(err)
			err = syscall.Mount(block, "/tmp/writable", "ext4", 0, "")
			rplib.Checkerr(err)
			defer syscall.Unmount("/tmp/writable", 0)
			rplib.Shellexec("tar", "--xattrs", "-xJvpf", "/recovery/factory/writable.tar.xz", "-C", "/tmp/writable/")
		case part.SysbootLabel:
			rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", part.SysbootLabel, block)
			err := os.MkdirAll("/tmp/system-boot/", 0755)
			rplib.Checkerr(err)
			err = syscall.Mount(block, "/tmp/system-boot", "vfat", 0, "")
			rplib.Checkerr(err)
			defer syscall.Unmount("/tmp/system-boot", 0)
			rplib.Shellexec("tar", "--xattrs", "-xJvpf", "/recovery/factory/system-boot.tar.xz", "-C", "/tmp/system-boot/")
			rplib.Shellexec("parted", "-ms", device, "set", strconv.Itoa(nr), "boot", "on")
		}
		last_end = new_end
		nr = nr + 1
	}
	return
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

func hack_uboot_env(uboot_cfg string) {
	env, err := uenv.Open(uboot_cfg)
	rplib.Checkerr(err)

	var name, value string
	//update env
	//1. mmcreco=1
	name = "mmcreco"
	value = "1"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//2. mmcpart=2
	name = "mmcpart"
	value = "2"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//3. snappy_boot
	name = "snappy_boot"
	value = "if test \"${snap_mode}\" = \"try\"; then setenv snap_mode \"trying\"; saveenv; if test \"${snap_try_core}\" != \"\"; then setenv snap_core \"${snap_try_core}\"; fi; if test \"${snap_try_kernel}\" != \"\"; then setenv snap_kernel \"${snap_try_kernel}\"; fi; elif test \"${snap_mode}\" = \"trying\"; then setenv snap_mode \"\"; saveenv; elif test \"${snap_mode}\" = \"recovery\"; then setenv loadinitrd \"load mmc ${mmcdev}:${mmcreco} ${initrd_addr} ${initrd_file}; setenv initrd_size ${filesize}\"; setenv loadkernel \"load mmc ${mmcdev}:${mmcreco} ${loadaddr} ${kernel_file}\"; setenv factory_recovery \"run loadfiles; setenv mmcroot \"/dev/disk/by-label/writable ${snappy_cmdline} snap_core=${snap_core} snap_kernel=${snap_kernel} recoverytype=factory_restore\"; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} 0x02000000\"; echo \"RECOVERY\"; run factory_recovery; fi; run loadfiles; setenv mmcroot \"/dev/disk/by-label/writable ${snappy_cmdline} snap_core=${snap_core} snap_kernel=${snap_kernel}\"; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} 0x02000000"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//4. loadbootenv (load uboot.env from system-boot, because snapd always update uboot.env in system-boot while os/kernel snap updated)
	name = "loadbootenv"
	value = "load ${devtype} ${devnum}:${mmcpart} ${loadaddr} ${bootenv}"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//5. bootenv (for system-boot/uboot.env)
	name = "bootenv"
	value = "uboot.env"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)
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
	var response string
	fmt.Scanf("%s\n", &response)
	ioutil.WriteFile("/proc/sys/kernel/printk", []byte("4 4 1 7"), 0644)

	if "y" != response && "Y" != response {
		return true
	}

	return false
}

func BackupWritable() {
	// back up serial assertion
	writable_part := rplib.Findfs("LABEL=writable")
	err := os.MkdirAll("/tmp/writable/", 0755)
	rplib.Checkerr(err)
	err = syscall.Mount(writable_part, "/tmp/writable/", "ext4", 0, "")
	rplib.Checkerr(err)
	// back up assertion if ever signed
	if _, err := os.Stat(filepath.Join("/tmp/", ASSERTION_FOLDER)); err == nil {
		rplib.Shellexec("cp", "-ar", filepath.Join("/tmp/", ASSERTION_FOLDER), ASSERTION_BACKUP_FOLDER)
	}
	syscall.Unmount("/tmp/writable", 0)
}

func GetBootDevName(RecoveryLabel string) (devNode string, devPath string, err error) {
	//devPath = rplib.Findfs(fmt.Sprintf("LABEL=%s", RecoveryLabel))
	cmd := exec.Command("findfs", fmt.Sprintf("LABEL=%s", RecoveryLabel))
	out, err := cmd.Output()
	if err != nil {
		return
	}
	devPath = strings.TrimSpace(string(out[:]))

	if strings.Contains(devPath, "/dev/") == false {
		err = errors.New(fmt.Sprintf("RecoveryLabel of %q not found", RecoveryLabel))
		return
	}

	// The devPath is with partiion /dev/sdX1 or /dev/mmcblkXp1
	// Here to remove the partition information
	for {
		if _, err := strconv.Atoi(string(devPath[len(devPath)-1])); err == nil {
			devPath = devPath[:len(devPath)-1]
		} else if devPath[len(devPath)-1] == 'p' {
			devPath = devPath[:len(devPath)-1]
			break
		} else {
			break
		}
	}

	field := strings.Split(devPath, "/")
	devNode = field[len(field)-1]

	return
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

	// TODO: verify the image
	// If this is user triggered factory restore (first time is in factory and should happen automatically), ask user for confirm.
	if rplib.FACTORY_RESTORE == RecoveryType {
		if ConfirmRecovry() == false {
			os.Exit(1)
		}

		//backup user data
		BackupWritable()
	}

	// Find boot device name
	_, BootDevPath, err := GetBootDevName(RecoveryLabel)
	if err != nil {
		logger.Panicf("Boot device not found, error: %s\n", err)
	}

	if configs.Configs.PartitionType == "gpt" {
		log.Println("[recover the backup GPT entry at end of the disk.]")
		rplib.Shellexec("sgdisk", BootDevPath, "--randomize-guids", "--move-second-header")
		log.Println("[recreate gpt partition table.]")
	}

	//Get system-boot, writable, recovery partition location
	parts, err := part.GetPartitions(BootDevPath, RecoveryLabel)
	fmt.Println(parts)

	// rebuild the partitions
	log.Println("[rebuild the partitions]")
	//recreateRecoveryPartition(BootDevPath, RecoveryLabel, recovery_nr, recovery_end)

	// stream log to stdout and writable partition
	writable_part := rplib.Findfs("LABEL=writable")
	err = os.MkdirAll("/tmp/writable/", 0755)
	rplib.Checkerr(err)
	err = syscall.Mount(writable_part, "/tmp/writable/", "ext4", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount("/tmp/writable", 0)
	rootdir := "/tmp/writable/system-data/"

	logfile := filepath.Join("/tmp/", LOG_PATH)
	err = os.MkdirAll(path.Dir(logfile), 0755)
	rplib.Checkerr(err)
	log_writable, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0600)
	rplib.Checkerr(err)
	f := io.MultiWriter(log_writable, os.Stdout)
	log.SetOutput(f)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// FIXME, if grub need to support

	log.Println("[Add snaps for oem]")
	os.MkdirAll(filepath.Join(rootdir, "/var/lib/oem/"), 0755)
	rplib.Shellexec("cp", "-a", "/recovery/factory/snaps", filepath.Join(rootdir, "/var/lib/oem/"))
	rplib.Shellexec("cp", "-a", "/recovery/factory/snaps-devmode", filepath.Join(rootdir, "/var/lib/oem/"))

	// add firstboot service
	const MULTI_USER_TARGET_WANTS_FOLDER = "/etc/systemd/system/multi-user.target.wants/"
	log.Println("[Add FIRSTBOOT service]")
	rplib.Shellexec("/recovery/bin/rsync", "-a", "--exclude='.gitkeep'", filepath.Join("/recovery/factory", RecoveryType)+"/", rootdir+"/")
	rplib.Shellexec("ln", "-s", "/lib/systemd/system/devmode-firstboot.service", filepath.Join(rootdir, MULTI_USER_TARGET_WANTS_FOLDER, "devmode-firstboot.service"))
	ioutil.WriteFile(filepath.Join(rootdir, "/var/lib/devmode-firstboot/conf.sh"), []byte(fmt.Sprintf("RECOVERYFSLABEL=\"%s\"\nRECOVERY_TYPE=\"%s\"\n", RecoveryLabel, RecoveryType)), 0644)

	switch RecoveryType {
	case rplib.FACTORY_INSTALL:
		log.Println("[EXECUTE FACTORY INSTALL]")

		log.Println("[set next recoverytype to factory_restore]")
		rplib.Shellexec("mount", "-o", "rw,remount", "/recovery_partition")

		log.Println("[Start serial vault]")
		interface_list := strings.Split(rplib.Shellcmdoutput("ip -o link show | awk -F': ' '{print $2}'"), "\n")
		log.Println("interface_list:", interface_list)

		var net = 0
		for ; net < len(interface_list); net++ {
			if strings.Contains(interface_list[net], "eth") == true || strings.Contains(interface_list[net], "enx") == true {
				break
			}
		}
		if net == len(interface_list) {
			panic(fmt.Sprintf("Need one ethernet interface to connect to identity-vault. Current network interface: %v", interface_list))
		}
		eth := interface_list[net] // select nethernet interface.
		rplib.Shellexec("ip", "link", "set", "dev", eth, "up")
		rplib.Shellexec("dhclient", "-1", eth)

		vaultServerIP := rplib.Shellcmdoutput("ip route | awk '/default/ { print $3 }'") // assume identity-vault is hosted on the gateway
		log.Println("vaultServerIP:", vaultServerIP)

		// TODO: read assertion information from gadget snap
		if !configs.Recovery.SignSerial {
			log.Println("Will not sign serial")
			break
		}
		// TODO: Start signing serial
		log.Println("Start signing serial")
	case rplib.FACTORY_RESTORE:
		log.Println("[User restores the system]")
		// restore assertion if ever signed
		if _, err := os.Stat(ASSERTION_BACKUP_FOLDER); err == nil {
			log.Println("Restore gpg key and serial")
			rplib.Shellexec("cp", "-ar", ASSERTION_BACKUP_FOLDER, filepath.Join("/tmp/", ASSERTION_FOLDER))
		}
	}

	// update uboot env
	system_boot_part := rplib.Findfs("LABEL=system-boot")
	log.Println("system_boot_part:", system_boot_part)
	err = os.MkdirAll("/tmp/system-boot", 0755)
	rplib.Checkerr(err)
	err = syscall.Mount(system_boot_part, "/tmp/system-boot", "vfat", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount("/tmp/system-boot", 0)

	log.Println("Update uboot env(ESP/system-boot)")
	//fsck needs ignore error code
	cmd := exec.Command("fsck", "-y", fmt.Sprintf("/dev/disk/by-label/%s", RecoveryLabel))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
	rplib.Shellexec("mount", "-o", "remount,rw", fmt.Sprintf("/dev/disk/by-label/%s", RecoveryLabel), "/recovery_partition")
	hack_uboot_env("/recovery_partition/uboot.env")
	rplib.Shellexec("mount", "-o", "remount,ro", fmt.Sprintf("/dev/disk/by-label/%s", RecoveryLabel), "/recovery_partition")
	hack_uboot_env("/tmp/system-boot/uboot.env")

	//release dhclient
	rplib.Shellexec("dhclient", "-x")
	rplib.Sync()
}
