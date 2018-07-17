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
	"regexp"
	"strings"
	"syscall"
	"time"

	uenv "github.com/mvo5/uboot-go/uenv"

	hooks "github.com/Lyoncore/ubuntu-custom-recovery/src/hooks"
	rplib "github.com/Lyoncore/ubuntu-custom-recovery/src/rplib"
)

const GRUB_MENUENTRY_FACTORY_RESTORE = `
menuentry "Factory Restore" {
		###OS_GRUB_MENU_CMDS###
        # load recovery system
        echo "[grub.cfg] load factory_restore system"
        search --no-floppy --set --label "###RECO_PARTITION_LABEL###"
        echo "[grub.cfg] root: ${root}"
		load_env -f (${root})/###EFI_DIR###/ubuntu/grubenv
        set cmdline="recovery=LABEL=###RECO_PARTITION_LABEL### ro init=/lib/systemd/systemd console=tty1 panic=-1 fixrtc -- recoverytype=factory_restore recoverylabel=###RECO_PARTITION_LABEL### snap_core=${recovery_core} snap_kernel=${recovery_kernel} recoveryos=###RECO_OS###"
        echo "[grub.cfg] loading kernel..."
        linuxefi ($root)/###RECO_BOOTIMG_PATH###kernel.img $cmdline
        echo "[grub.cfg] loading initrd..."
        initrdefi ($root)/###RECO_BOOTIMG_PATH###initrd.img
        echo "[grub.cfg] boot..."
        boot
}
`

const UBUNTU_CORE_GRUB_MENU_CMDS = ``
const RECO_UBUNTU_CORE = `ubuntu_core`
const RECO_BOOTIMG_PATH_UBUNTU_CORE = `$recovery_kernel/`

const UBUNTU_CLASSIC_GRUB_MENU_CMDS = `
        recordfail
        load_video
        gfxmode auto
        insmod gzio
        if [ x$grub_platform = xxen ]; then insmod xzio; insmod lzopio; fi
        insmod part_gpt
        insmod ext2`
const RECO_UBUNTU_CLASSIC = `ubuntu_classic`
const RECO_BOOTIMG_PATH_UBUNTU_CLASSIC = ``

func UpdateGrubCfg(recovery_part_label string, grub_cfg string, grub_env string, recoveryos string) error {
	rplib.Shellexec("sed", "-i", "s/^set cmdline=\"\\(.*\\)\"$/set cmdline=\"\\1 $cloud_init_disabled\"/g", grub_cfg)

	// add recovery grub menuentry
	f, err := os.OpenFile(grub_cfg, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("Open %s failed", grub_cfg)
		return err
	}
	defer f.Close()

	var menuentry string
	if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
		menuentry = strings.Replace(GRUB_MENUENTRY_FACTORY_RESTORE, "###OS_GRUB_MENU_CMDS###", UBUNTU_CLASSIC_GRUB_MENU_CMDS, -1)
		menuentry = strings.Replace(menuentry, "###EFI_DIR###", Efi_dir, -1)
		menuentry = strings.Replace(menuentry, "###RECO_OS###", RECO_UBUNTU_CLASSIC, -1)
		menuentry = strings.Replace(menuentry, "###RECO_BOOTIMG_PATH###", RECO_BOOTIMG_PATH_UBUNTU_CLASSIC, -1)
	} else if recoveryos == rplib.RECOVERY_OS_UBUNTU_CORE {
		menuentry = strings.Replace(GRUB_MENUENTRY_FACTORY_RESTORE, "###OS_GRUB_MENU_CMDS###", UBUNTU_CORE_GRUB_MENU_CMDS, -1)
		menuentry = strings.Replace(menuentry, "###EFI_DIR###", Efi_dir, -1)
		menuentry = strings.Replace(menuentry, "###RECO_OS###", RECO_UBUNTU_CORE, -1)
		menuentry = strings.Replace(menuentry, "###RECO_BOOTIMG_PATH###", RECO_BOOTIMG_PATH_UBUNTU_CORE, -1)
	}
	menuentry = strings.Replace(menuentry, "###RECO_PARTITION_LABEL###", recovery_part_label, -1)

	if _, err = f.WriteString(menuentry); err != nil {
		return err
	}

	cmd := exec.Command("grub-editenv", grub_env, "set", "recovery_type=factory_restore")
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func UpdateUbootEnv(RecoveryLabel string) error {
	// update uboot.env in recovery partition after first install
	env, err := uenv.Open(SYSBOOT_UBOOT_ENV)
	if err != nil {
		log.Printf("Open %s failed", SYSBOOT_UBOOT_ENV)
		return err
	}

	env.Set("snap_mode", "")
	if err = env.Save(); err != nil {
		log.Printf("Write %s failed", SYSBOOT_UBOOT_ENV)
		return err
	}

	env.Set("recovery_type", "factory_restore")
	if err = env.Save(); err != nil {
		log.Printf("Write %s failed", SYSBOOT_UBOOT_ENV)
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
		log.Printf("Write %s failed", SYSBOOT_UBOOT_ENV)
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
		log.Printf("Write %s failed", SYSBOOT_UBOOT_ENV)
		return err
	}

	env.Set("recovery_label", fmt.Sprintf("LABEL=%s", RecoveryLabel))
	if err = env.Save(); err != nil {
		log.Printf("Write %s failed", SYSBOOT_UBOOT_ENV)
		return err
	}
	return err
}

func UpdateFstab(parts *Partitions, recoveryos string) error {
	if parts == nil {
		return fmt.Errorf("nil Partitions")
	}

	if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
		writable_uuid := rplib.Shellcmdoutput(fmt.Sprintf("blkid -s UUID -o value %s", fmtPartPath(parts.TargetDevPath, parts.Writable_nr)))
		match, err := regexp.MatchString("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", writable_uuid)
		if err != nil || match == false {
			return fmt.Errorf("finding writable uuid failed:%v", writable_uuid)
		}
		sysboot_uuid := rplib.Shellcmdoutput(fmt.Sprintf("blkid -s UUID -o value %s", fmtPartPath(parts.TargetDevPath, parts.Sysboot_nr)))
		match, err = regexp.MatchString("[0-9a-fA-F]{4}-[0-9a-fA-F]{4}", sysboot_uuid)
		if err != nil || match == false {
			return fmt.Errorf("finding system-boot uuid failed:%v", sysboot_uuid)
		}

		f_fstab, err := os.Create(WRITABLE_ETC_FSTAB)
		if err != nil {
			return err
		}
		defer f_fstab.Close()

		_, err = f_fstab.WriteString(fmt.Sprintf("UUID=%v	/	ext4	errors=remount-ro	0	1\n", writable_uuid))
		if err != nil {
			return err
		}
		_, err = f_fstab.WriteString(fmt.Sprintf("UUID=%v	/boot/%s	vfat	umask=0077	0	1\n", sysboot_uuid, Efi_dir))
		if err != nil {
			return err
		}
	}

	return nil
}

func chrootWritablePrepare(writableMnt string, sysbootMnt string) error {
	var efiMnt = filepath.Join(writableMnt, "boot", Efi_dir)
	if _, err := os.Stat(efiMnt); os.IsNotExist(err) {
		if err = os.Mkdir(efiMnt, 0755); err != nil {
			return err
		}

	}

	if err := syscall.Mount(sysbootMnt, efiMnt, "vfat", syscall.MS_BIND, ""); err != nil {
		return err
	}

	if err := syscall.Mount("/sys", filepath.Join(writableMnt, "sys"), "sysfs", syscall.MS_BIND, ""); err != nil {
		return err
	}

	if err := syscall.Mount("/proc", filepath.Join(writableMnt, "proc"), "proc", syscall.MS_BIND, ""); err != nil {
		return err
	}

	if err := syscall.Mount("/dev", filepath.Join(writableMnt, "dev"), "devtmpfs", syscall.MS_BIND, ""); err != nil {
		return err
	}

	if err := syscall.Mount("/run", filepath.Join(writableMnt, "run"), "tmpfs", syscall.MS_BIND, ""); err != nil {
		return err
	}

	return nil
}

func chrootUmountBinded(writableMnt string) error {
	if err := syscall.Unmount(filepath.Join(writableMnt, "boot", Efi_dir), 0); err != nil {
		return err
	}

	if err := syscall.Unmount(filepath.Join(writableMnt, "sys"), 0); err != nil {
		return err
	}

	if err := syscall.Unmount(filepath.Join(writableMnt, "proc"), 0); err != nil {
		return err
	}

	if err := syscall.Unmount(filepath.Join(writableMnt, "dev"), 0); err != nil {
		return err
	}

	if err := syscall.Unmount(filepath.Join(writableMnt, "run"), 0); err != nil {
		return err
	}

	return nil
}

func GrubInstall(writableMnt string, sysbootMnt string, recoveryos string, displayGrubMenu bool, swapenable bool, resumeDev string) error {
	if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
		// Remove old entries and recreate
		recov_entry := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
		os_boot_entry := rplib.GetBootEntries(rplib.RECOVERY_OS_UBUNTU_CLASSIC)
		if len(recov_entry) < 1 || len(os_boot_entry) < 1 {
			//remove old uefi entry
			entries := rplib.GetBootEntries(rplib.BOOT_ENTRY_RECOVERY)
			for _, entry := range entries {
				rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
			}
			entries = rplib.GetBootEntries(rplib.RECOVERY_OS_UBUNTU_CLASSIC)
			for _, entry := range entries {
				rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
			}
			// add new uefi entry
			log.Println("[add new uefi entry]")
			rplib.CreateBootEntry(parts.TargetDevPath, parts.Recovery_nr, LOADER, rplib.BOOT_ENTRY_RECOVERY)
		}

		if err := chrootWritablePrepare(writableMnt, sysbootMnt); err != nil {
			return err
		}
		defer chrootUmountBinded(writableMnt)

		if displayGrubMenu {
			rplib.Shellexec("sed", "-i", "s/^GRUB_HIDDEN_TIMEOUT=0/GRUB_RECORDFAIL_TIMEOUT=3\\n#GRUB_HIDDEN_TIMEOUT=0/g", filepath.Join(writableMnt, "etc/default/grub"))
		}

		if swapenable {
			rplib.Shellexec("sed", "-i", fmt.Sprintf("s@quiet splash@quiet splash resume=%s@g", resumeDev), filepath.Join(writableMnt, "etc/default/grub"))
		}

		//Remove all old grub in boot partition if exist
		d, err := os.Open(sysbootMnt)
		if err != nil {
			return err
		}
		defer d.Close()
		names, err := d.Readdirnames(-1)
		if err != nil {
			return err
		}
		for _, name := range names {
			err = os.RemoveAll(filepath.Join(sysbootMnt, name))
			if err != nil {
				return err
			}
		}

		rplib.Shellexec("chroot", writableMnt, "grub-install", "--target=x86_64-efi")

		rplib.Shellexec("chroot", writableMnt, "update-grub")

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

func ConfirmRecovery(timeout int64, recoveryos string) bool {
	const (
		msg1         = "Factory Restore: "
		msg2         = "Factory Restore will delete all user data, are you sure? [y/N] "
		msg3         = "(press [y] + [enter] to confirm) "
		event_start  = "start"
		event_finish = "finish"
		curtin_yaml  = "/var/log/installer/subiquity-curtin-install.conf"
	)

	if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC {
		rplib.Shellexec("plymouth", "hide-splash")
	}
	ioutil.WriteFile("/proc/sys/kernel/printk", []byte("0 0 0 0"), 0644)

	if configs.Recovery.RestoreConfirmPrehookFile != "" {
		hooks.RestoreConfirmPrehook.SetPath(HOOKS_DIR + configs.Recovery.RestoreConfirmPrehookFile)
		if hooks.RestoreConfirmPrehook.IsHookExist() {
			err := hooks.RestoreConfirmPrehook.Run(RECO_ROOT_DIR, false, "", "")
			if err != nil {
				log.Println(err)
			}
		}
	}
	log.Println("Wait user confirmation timeout:", timeout, "sec")
	log.Println("Factory Restore will delete all user data, are you sure? [y/N] ")

	tty, err := os.Open("/dev/tty1")
	if err != nil {
		panic(err)
	}

	// disable input buffering
	exec.Command("stty", "-F", "/dev/tty1", "cbreak", "min", "1").Run()
	// do not display entered characters on the screen
	exec.Command("stty", "-F", "/dev/tty1", "-echo").Run()
	defer exec.Command("stty", "-F", "/dev/tty1", "echo").Run()

	var b []byte = make([]byte, 1)
	response := make(chan []byte)
	go func() {
		for {
			tty.Read(b)
			if string(b) == "y" || string(b) == "Y" || string(b) == "n" || string(b) == "N" {
				response <- b
			}
		}
	}()

	select {
	case s := <-response:
		log.Println("response:", string(s))
	case <-time.After(time.Second * time.Duration(timeout)):
		log.Println("Timeout:", timeout, "sec. Reboot system!")
	}

	ioutil.WriteFile("/proc/sys/kernel/printk", []byte("4 4 1 7"), 0644)
	if configs.Recovery.RestoreConfirmPosthookFile != "" {
		hooks.RestoreConfirmPosthook.SetPath(HOOKS_DIR + configs.Recovery.RestoreConfirmPosthookFile)
	}

	if "y" != string(b) && "Y" != string(b) {
		if hooks.RestoreConfirmPosthook.IsHookExist() {
			err := hooks.RestoreConfirmPosthook.Run(RECO_ROOT_DIR, true, "USERCONFIRM", "no")
			if err != nil {
				log.Println(err)
			}
		}
		return false
	}

	if hooks.RestoreConfirmPosthook.IsHookExist() {
		err := hooks.RestoreConfirmPosthook.Run(RECO_ROOT_DIR, true, "USERCONFIRM", "yes")
		if err != nil {
			log.Println(err)
		}
	}
	return true
}

func BackupAssertions(parts *Partitions) error {
	if parts == nil {
		fmt.Errorf("nil Partitions")
	}

	// back up serial assertion
	err := os.MkdirAll(WRITABLE_MNT_DIR, 0755)
	if err != nil {
		return err
	}
	err = syscall.Mount(fmtPartPath(parts.TargetDevPath, parts.Writable_nr), WRITABLE_MNT_DIR, "ext4", 0, "")
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
			log.Println(err)
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

func EnableLogger(logPath string) error {
	if _, err := os.Stat(path.Dir(logPath)); err != nil {
		err = os.MkdirAll(path.Dir(logPath), 0755)
		if err != nil {
			return err
		}
	}
	log_writable, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0600)
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

	stat, err := os.Stat(FIRSTBOOT_SERVICE_DIR)
	if err == nil && stat.IsDir() == true {
		err := ioutil.WriteFile(filepath.Join(SYSTEM_DATA_PATH, FIRSTBOOT_SREVICE_SCRIPT), []byte(fmt.Sprintf("RECOVERYFSLABEL=\"%s\"\nRECOVERY_TYPE=\"%s\"\n", RecoveryLabel, RecoveryType)), 0644)
		if err != nil {
			return err
		}
	}

	return nil
}
