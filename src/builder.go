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
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	uenv "github.com/mvo5/uboot-go/uenv"

	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
)

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

func UpdateUbootEnv() error {
	// update uboot.env in recovery partition after first install
	env, err := uenv.Open(UBOOT_ENV)
	if err != nil {
		log.Println("Open %s failed", UBOOT_ENV)
		return err
	}

	env.Set("snap_mode", "")
	if err = env.Save(); err != nil {
		log.Println("Write %s failed", UBOOT_ENV)
		return err
	}

	env.Set("recovery_type", "factory_restore")
	if err = env.Save(); err != nil {
		log.Println("Write %s failed", UBOOT_ENV)
		return err
	}

	var core, kernel string
	core_s, _ := filepath.Glob(BACKUP_SNAP_PATH + "*core*.snap")
	if cap(core_s) == 1 {
		core = filepath.Base(strings.Join(core_s, ""))
	} else {
		log.Println("Error! no core snap or too many found:", core_s)
		return fmt.Errorf("Finding core snap error in %s", BACKUP_SNAP_PATH)
	}
	env.Set("recovery_core", core)
	if err = env.Save(); err != nil {
		log.Println("Write %s failed", UBOOT_ENV)
		return err
	}

	kernel_s, _ := filepath.Glob(BACKUP_SNAP_PATH + "*kernel*.snap")
	if cap(kernel_s) == 1 {
		kernel = filepath.Base(strings.Join(kernel_s, ""))
	} else {
		log.Println("Error! no kernel snap or too many found:", kernel_s)
		return fmt.Errorf("Finding kernel snap error in %s", BACKUP_SNAP_PATH)
	}
	env.Set("recovery_kernel", kernel)
	if err = env.Save(); err != nil {
		log.Println("Write %s failed", UBOOT_ENV)
		return err
	}
	return err
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

func BackupAssertions(parts *Partitions) error {
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

func CopySnapsAsserts() error {
	if _, err := os.Stat(SNAPS_DST_PATH); err != nil {
		err = os.MkdirAll(SNAPS_DST_PATH, 0755)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(SNAPS_SRC_PATH); err == nil {
		err = rplib.CopyTree(SNAPS_SRC_PATH, SNAPS_DST_PATH)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(DEV_SNAPS_SRC_PATH); err == nil {
		err = rplib.CopyTree(DEV_SNAPS_SRC_PATH, SNAPS_DST_PATH)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(ASSERT_PRE_SRC_PATH); err == nil {
		err = rplib.CopyTree(ASSERT_PRE_SRC_PATH, ASSERT_DST_PATH)
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

	// unpack writable-includes
	cmd := exec.Command("unsquashfs", "-f", "-d", WRITABLE_MNT_DIR, WRITABLE_INCLUDES_SQUASHFS)
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
