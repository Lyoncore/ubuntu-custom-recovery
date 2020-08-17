/*
 * Copyright (C) 2016 Canonical Ltd
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
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	rplib "github.com/Lyoncore/ubuntu-custom-recovery/src/rplib"
)

/*            The partiion layout
 *
 *               u-boot system
 * _________________________________________
 *|                                         |
 *|             GPT/MBR table               |
 *|_________________________________________|
 *|     (Maybe bootloader W/O partitions)   |
 *|           Part 1 (bootloader)           |
 *|_________________________________________|
 *|                                         |
 *|   Part 2 (bootloader or other raw data) |
 *|_________________________________________|
 *|    Part ... (maybe more for raw data)   |
 *|-----------------------------------------|
 *|-----------------------------------------|
 *|         Part X-1 (system-boot)          |
 *|_________________________________________|
 *|                                         |
 *|            Part X (Recovery)            |
 *|_________________________________________|
 *|                                         |
 *|            Part X+1 (writable)          |
 *|_________________________________________|
 *
 *
 *               grub system
 * _________________________________________
 *|                                         |
 *|              GPT/MBR table              |
 *|_________________________________________|
 *|                                         |
 *|            Part 1 (Recovery)            |
 *|_________________________________________|
 *|                                         |
 *|           Part 2 (system-boot)          |
 *|_________________________________________|
 *|                                         |
 *|            Part 3 (writable)            |
 *|_________________________________________|
 */

type Partitions struct {
	// XxxDevNode: sda (W/O partiiton number)
	// XxxDevPath: /dev/sda (W/O partition number)
	SourceDevNode, SourceDevPath                                string
	TargetDevNode, TargetDevPath                                string
	Recovery_nr, Sysboot_nr, Swap_nr, Writable_nr, Last_part_nr int
	Recovery_start, Recovery_end                                int64
	Sysboot_start, Sysboot_end                                  int64
	Swap_start, Swap_end                                        int64
	Writable_start, Writable_end                                int64
	TargetSize                                                  int64
}

const (
	SysbootLabel  = "system-boot"
	WritableLabel = "writable"
	SwapLabel     = "swap"
)

func FindPart(Label string) (devNode string, devPath string, partNr int, err error) {
	partNr = -1
	cmd := exec.Command("findfs", fmt.Sprintf("LABEL=%s", Label))
	out, err := cmd.Output()
	if err != nil {
		return
	}
	fullPath := strings.TrimSpace(string(out[:]))

	if strings.Contains(fullPath, "/dev/") == false {
		err = errors.New(fmt.Sprintf("Label of %q not found", Label))
		return
	}
	devPath = fullPath

	// The devPath is with partiion /dev/sdX1 or /dev/mmcblkXp1
	// Here to remove the partition information
	for {
		if _, err := strconv.Atoi(string(devPath[len(devPath)-1])); err == nil {
			devPath = devPath[:len(devPath)-1]
		} else {
			part_nr := strings.TrimPrefix(fullPath, devPath)
			if partNr, err = strconv.Atoi(part_nr); err != nil {
				err = errors.New("Unknown error while FindPart")
				return "", "", -1, err
			}
			if devPath[len(devPath)-1] == 'p' {
				devPath = devPath[:len(devPath)-1]
			}
			break
		}
	}

	field := strings.Split(devPath, "/")
	devNode = field[len(field)-1]

	return
}

func FindTargetParts(parts *Partitions, recoveryType string) error {
	if parts.SourceDevNode == "" || parts.SourceDevPath == "" || parts.Recovery_nr == -1 {
		return fmt.Errorf("Missing source recovery data")
	}

	// If config.yaml has set the specific system device,
	// it would use is as system device.
	// Or it would assume the system device is same as recovery device
	if configs.Recovery.SystemDevice != "" {
		parts.TargetDevPath = configs.Recovery.SystemDevice
		parts.TargetDevNode = filepath.Base(parts.TargetDevPath)
	} else {
		parts.TargetDevNode = parts.SourceDevNode
		parts.TargetDevPath = parts.SourceDevPath
	}
	return nil
}

var parts Partitions

func GetPartitions(recoveryLabel string, recoveryType string) (*Partitions, error) {
	var err error
	const OLD_PARTITION = "/tmp/old-partition.txt"
	parts = Partitions{"", "", "", "", -1, -1, -1, -1, -1, 0, 20479, -1, -1, -1, -1, -1, -1, -1}

	//The Sourec device which must has a recovery partition
	parts.SourceDevNode, parts.SourceDevPath, parts.Recovery_nr, err = FindPart(recoveryLabel)
	if err != nil {
		err = errors.New(fmt.Sprintf("Recovery partition (LABEL=%s) not found", recoveryLabel))
		return nil, err
	}

	err = FindTargetParts(&parts, recoveryType)
	if err != nil {
		err = errors.New(fmt.Sprintf("Target install partition not found"))
		parts = Partitions{"", "", "", "", -1, -1, -1, -1, -1, 0, 20479, -1, -1, -1, -1, -1, -1, -1}
		return nil, err
	}

	//system-boot partition info
	_, _, sysboot_nr, err := FindPart(SysbootLabel)
	if err == nil {
		parts.Sysboot_nr = sysboot_nr
	}

	//swap partition info
	_, _, parts.Swap_nr, err = FindPart(SwapLabel)
	if err != nil {
		//Partition not found, keep value in '-1'
		parts.Swap_nr = -1
	}

	//writable-boot partition info
	_, _, parts.Writable_nr, err = FindPart(WritableLabel)
	if err != nil {
		//Partition not found, keep value in '-1'
		parts.Writable_nr = -1
	}

	if parts.Recovery_nr == -1 && parts.Sysboot_nr == -1 && parts.Writable_nr == -1 {
		//Noting to find, return.
		return &parts, nil
	}

	// find out detail information of each partition
	cmd := exec.Command("parted", "-ms", fmt.Sprintf("/dev/%s", parts.TargetDevNode), "unit", "B", "print")
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")

		// get disk size
		if strings.Contains(fields[0], "/dev/") == true {
			parts.TargetSize, err = strconv.ParseInt(strings.TrimRight(fields[1], "B"), 10, 64)
			if err != nil {
				fmt.Errorf("Parsing disk size failed")
			}
			continue
		}

		nr, err := strconv.Atoi(fields[0])
		if err != nil { //ignore the line don't neeed
			continue
		}
		end, err := strconv.ParseInt(strings.TrimRight(fields[2], "B"), 10, 64)

		if err != nil {
			return nil, err
		}

		start, err := strconv.ParseInt(strings.TrimRight(fields[1], "B"), 10, 64)

		if err != nil {
			return nil, err
		}

		if parts.SourceDevPath == parts.TargetDevPath {
			if parts.Recovery_nr != -1 && parts.Recovery_nr == nr {
				parts.Recovery_start = start
				parts.Recovery_end = end
			}
		}
		parts.Last_part_nr = nr
	}
	cmd.Wait()
	err = scanner.Err()
	if err != nil {
		return nil, err
	}
	return &parts, nil
}

func SetPartitionStartEnd(parts *Partitions, partName string, partSizeMB int, bootloader string) error {
	if parts == nil {
		return fmt.Errorf("nil Partitions")
	}

	switch partName {
	case "system-boot":
		if bootloader == "u-boot" {
			// Not allow to edit system-boot in u-boot yet.
		} else if bootloader == "grub" {
			// mmcblk0p2 start = (1077936127B + 1) / 1024 / 1024 = 1028MiB
			parts.Sysboot_start = (parts.Recovery_end + 1) / (1024 * 1024)
			// mmcblk0p2 end = 1028MiB + 512MiB = 1540 MiB
			parts.Sysboot_end = parts.Sysboot_start + int64(partSizeMB)
		}
	case "swap":
		if bootloader == "u-boot" {
			// Not allow to edit swap in u-boot yet.
		} else if bootloader == "grub" {
			// mmcblk0p3 start = 1540MiB
			parts.Swap_start = parts.Sysboot_end
			// mmcblk0p3 end = 1540MiB + 1024MiB = 2564MiB
			parts.Swap_end = parts.Swap_start + int64(partSizeMB)
		}
		// The writable partition would be enlarged to maximum.
		// Here does not support change the Start, End
	default:
		return fmt.Errorf("Unknown Partition Name %s", partName)
	}

	return nil
}

func CopyRecoveryPart(parts *Partitions) error {
	if parts.SourceDevPath == parts.TargetDevPath {
		return fmt.Errorf("The source device and target device are same")
	}

	parts.Recovery_nr = 1
	recoveryBegin := 4
	if configs.Recovery.RecoverySize <= 0 {
		return fmt.Errorf("Invalid recovery size: %d", configs.Recovery.RecoverySize)
	}
	recoveryEnd := recoveryBegin + configs.Recovery.RecoverySize

	// Build Recovery Partition
	recovery_path := fmtPartPath(parts.TargetDevPath, parts.Recovery_nr)
	rplib.Shellexec("parted", "-ms", "-a", "optimal", parts.TargetDevPath,
		"unit", "MiB",
		"mklabel", "gpt",
		"mkpart", "primary", "fat32", fmt.Sprintf("%d", recoveryBegin), fmt.Sprintf("%d", recoveryEnd),
		"name", fmt.Sprintf("%v", parts.Recovery_nr), configs.Recovery.FsLabel,
		"set", fmt.Sprintf("%v", parts.Recovery_nr), "boot", "on",
		"print")
	exec.Command("partprobe").Run()
	rplib.Shellexec("sleep", "2") //wait the partition presents
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", configs.Recovery.FsLabel, recovery_path)

	// Copy recovery data
	err := os.MkdirAll(RECO_TAR_MNT_DIR, 0755)
	if err != nil {
		return err
	}
	err = syscall.Mount(recovery_path, RECO_TAR_MNT_DIR, "vfat", 0, "")
	if err != nil {
		return err
	}
	defer syscall.Unmount(RECO_TAR_MNT_DIR, 0)
	rplib.Shellcmd(fmt.Sprintf("cd %s ; cp -a `ls | grep -v NvVars` %s", RECO_ROOT_DIR, RECO_TAR_MNT_DIR))
	rplib.Shellexec("sync")

	// set target grubenv to factory_restore
	if _, err = os.Stat(SYSBOOT_MNT_DIR + "EFI"); err == nil {
		cmd := exec.Command("grub-editenv", filepath.Join(RECO_TAR_MNT_DIR, "EFI/ubuntu/grubenv"), "set", "recovery_type=factory_install")
		cmd.Run()
	} else if _, err = os.Stat(SYSBOOT_MNT_DIR + "efi"); err == nil {
		cmd := exec.Command("grub-editenv", filepath.Join(RECO_TAR_MNT_DIR, "efi/ubuntu/grubenv"), "set", "recovery_type=factory_install")
		cmd.Run()
	}

	return nil
}

func RestoreParts(parts *Partitions, bootloader string, partType string, recoveryos string) error {
	var dev_path string = strings.Replace(parts.TargetDevPath, "mapper/", "", -1)
	part_nr := parts.Last_part_nr
	if bootloader == "u-boot" {
		parts.Writable_nr = parts.Recovery_nr + 1 //writable is one after recovery
	} else if bootloader == "grub" {
		if parts.SourceDevPath == parts.TargetDevPath {
			parts.Sysboot_nr = parts.Recovery_nr + 1
		} else {
			parts.Sysboot_nr = 1 //If target device is not same as source, the system-boot parition will start from 1st partition
		}
		if configs.Configs.Swap == true && configs.Configs.SwapFile != true && configs.Configs.SwapSize > 0 {
			parts.Swap_nr = parts.Sysboot_nr + 1  //swap is one after system-boot
			parts.Writable_nr = parts.Swap_nr + 1 //writable is one after swap
		} else {
			parts.Swap_nr = -1                       //swap is not enabled
			parts.Writable_nr = parts.Sysboot_nr + 1 //writable is one after system-boot
		}
	} else {
		return fmt.Errorf("Oops, unknown bootloader:%s", bootloader)
	}

	if partType == "gpt" {
		rplib.Shellexec("sgdisk", dev_path, "--randomize-guids", "--move-second-header")
	} else if partType == "mbr" {
		//nothing to do here
	} else {
		return fmt.Errorf("Oops, unknown partition type:%s", partType)
	}

	// Remove partitions expect the partitions before recovery
	if parts.SourceDevPath == parts.TargetDevPath {
		for part_nr > parts.Recovery_nr {
			rplib.Shellexec("parted", "-ms", dev_path, "rm", fmt.Sprintf("%v", part_nr))
			part_nr--
		}
	} else {
		// Build a new GPT to remove all partitions if target device is another disk
		rplib.Shellexec("parted", "-ms", dev_path, "mklabel", "gpt")
	}

	// Restore system-boot
	sysboot_path := fmtPartPath(parts.TargetDevPath, parts.Sysboot_nr)
	if bootloader == "u-boot" {
		// In u-boot, it keeps system-boot partition, and only mkfs
		if parts.Sysboot_nr == -1 {
			// oops, don't known the location of system-boot.
			// In the u-boot, system-boot would be in fron of recovery partition
			// If we lose system-boot, and we cannot know the proper location
			return fmt.Errorf("Oops, We lose system-boot")
		}
	} else if bootloader == "grub" {
		if partType == "gpt" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "fat32", fmt.Sprintf("%vMiB", parts.Sysboot_start), fmt.Sprintf("%vMiB", parts.Sysboot_end), "name", fmt.Sprintf("%v", parts.Sysboot_nr), SysbootLabel)
		} else if partType == "mbr" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "fat32", fmt.Sprintf("%vMiB", parts.Sysboot_start), fmt.Sprintf("%vMiB", parts.Sysboot_end))
		}
	}
	rplib.Shellexec("udevadm", "settle")

	exec.Command("partprobe").Run()
	rplib.Shellexec("sleep", "2") //wait the partition presents
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", SysbootLabel, sysboot_path)
	err := os.MkdirAll(SYSBOOT_MNT_DIR, 0755)
	if err != nil {
		return err
	}
	err = syscall.Mount(sysboot_path, SYSBOOT_MNT_DIR, "vfat", 0, "")
	if err != nil {
		return err
	}
	defer syscall.Unmount(SYSBOOT_MNT_DIR, 0)

	// The ubuntu classic would install grub by grub-install
	// If the sysboot tarball file not exists, just ignore it
	if _, err := os.Stat(SYSBOOT_TARBALL); !os.IsNotExist(err) {
		if err := os.MkdirAll("/tmp/tmp", 0755); err != nil {
			return err
		}
		rplib.Shellexec("tar", "-xpJvf", SYSBOOT_TARBALL, "-C", "/tmp/tmp")
		rplib.Shellexec("cp", "-r", "/tmp/tmp/.", SYSBOOT_MNT_DIR)
		rplib.Shellexec("rm", "-rf", "/tmp/tmp/")
	}
	rplib.Shellexec("parted", "-ms", dev_path, "set", strconv.Itoa(parts.Sysboot_nr), "boot", "on")

	// Create swap partition
	if configs.Configs.Swap == true && configs.Configs.SwapFile != true && configs.Configs.SwapSize > 0 {
		if partType == "gpt" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "linux-swap", fmt.Sprintf("%vMiB", parts.Swap_start), fmt.Sprintf("%vMiB", parts.Swap_end), "name", fmt.Sprintf("%v", parts.Swap_nr), SwapLabel)
		} else if partType == "mbr" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "linux-swap", fmt.Sprintf("%vMiB", parts.Swap_start), fmt.Sprintf("%vMiB", parts.Swap_end))
		}
		rplib.Shellexec("udevadm", "settle")
		rplib.Shellexec("mkswap", fmtPartPath(parts.TargetDevPath, parts.Swap_nr))
	}

	// Restore writable
	if configs.Configs.Swap == true && configs.Configs.SwapFile != true && configs.Configs.SwapSize > 0 {
		parts.Writable_start = parts.Swap_end
	} else {
		parts.Writable_start = parts.Sysboot_end
	}
	var writable_start string = fmt.Sprintf("%vMiB", parts.Writable_start)
	var writable_nr string = strconv.Itoa(parts.Writable_nr)
	writable_path := fmtPartPath(parts.TargetDevPath, parts.Writable_nr)

	if partType == "gpt" {
		rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "ext4", writable_start, "-1M", "name", writable_nr, WritableLabel)
	} else if partType == "mbr" {
		rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "ext4", writable_start, "-1M")
	}

	rplib.Shellexec("udevadm", "settle")
	exec.Command("partprobe").Run()

	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, writable_path)
	if recoveryos == rplib.RECOVERY_OS_UBUNTU_CLASSIC_CURTIN {
		// Curtin will handle the partition mounting and partition restore
		err := generateCurtinConf(parts)
		rplib.Checkerr(err)
		err = runCurtin()
		rplib.Checkerr(err)
		// If the image is deployed by maas, there will be a curtin/ directory in recovery partition
		// The nocloud-net config files are not needed, which using maas cloud-init config files
		// Or we write a nocloud-net config for user config, hostname config ... etc
		if _, err = os.Stat(RECO_ROOT_DIR + "curtin/"); os.IsNotExist(err) {
			err = writeCloudInitConf(parts)
			rplib.Checkerr(err)
		}
		return nil
	} else {
		err = os.MkdirAll(WRITABLE_MNT_DIR, 0755)
		rplib.Checkerr(err)
		err = syscall.Mount(writable_path, WRITABLE_MNT_DIR, "ext4", 0, "")
		rplib.Checkerr(err)
		defer syscall.Unmount(WRITABLE_MNT_DIR, 0)
		// Here to support install rootfs from squashfs file
		// If the writable tarball file not exists, just ignore it and unsquashfs the squashfs file
		if _, err := os.Stat(WRITABLE_TARBALL); !os.IsNotExist(err) {
			rplib.Shellexec("tar", "--xattrs", "-xJvpf", WRITABLE_TARBALL, "-C", WRITABLE_MNT_DIR)
		} else if _, err := os.Stat(ROOTFS_SQUASHFS); !os.IsNotExist(err) {
			rplib.Shellexec("unsquashfs", "-d", WRITABLE_MNT_DIR, "-f", ROOTFS_SQUASHFS)
		}
	}

	return nil
}
