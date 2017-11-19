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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"
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
	SourceDevNode, SourceDevPath                       string
	TargetDevNode, TargetDevPath                       string
	Recovery_nr, Sysboot_nr, Writable_nr, Last_part_nr int
	Recovery_start, Recovery_end                       int64
	Sysboot_start, Sysboot_end                         int64
	Writable_start, Writable_end                       int64
}

const (
	SysbootLabel  = "system-boot"
	WritableLabel = "writable"
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
			part_nr := strings.Trim(fullPath, devPath)
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
	var devPath string
	if parts.SourceDevNode == "" || parts.SourceDevPath == "" || parts.Recovery_nr == -1 {
		return fmt.Errorf("Missing source recovery data")
	}

	if recoveryType == rplib.HEADLESS_INSTALLER {
		// target disk might be emmc
		blockArray, _ := filepath.Glob("/sys/block/mmcblk*")
		for _, block := range blockArray {
			dat := []byte("")
			dat, err := ioutil.ReadFile(filepath.Join(block, "dev"))
			if err != nil {
				return err
			}
			dat_str := strings.TrimSpace(string(dat))
			blockDevice := rplib.Realpath(fmt.Sprintf("/dev/block/%s", dat_str))
			if blockDevice != parts.SourceDevPath {
				devPath = blockDevice
				break
			}
		}

		// target disk might be scsi disk
		if devPath == "" {
			blockArray, _ := filepath.Glob("/sys/block/sd*")
			for _, block := range blockArray {
				dat := []byte("")
				dat, err := ioutil.ReadFile(filepath.Join(block, "dev"))
				if err != nil {
					return err
				}
				dat_str := strings.TrimSpace(string(dat))
				blockDevice := rplib.Realpath(fmt.Sprintf("/dev/block/%s", dat_str))

				if blockDevice != parts.SourceDevPath {
					devPath = blockDevice
					break
				}
			}
		}

		if devPath != "" {
			// The devPath is with partiion /dev/sdX1 or /dev/mmcblkXp1
			// Here to remove the partition information
			for {
				if _, err := strconv.Atoi(string(devPath[len(devPath)-1])); err == nil {
					devPath = devPath[:len(devPath)-1]
				} else {
					if devPath[len(devPath)-1] == 'p' {
						devPath = devPath[:len(devPath)-1]
					}
					parts.TargetDevPath = devPath
					parts.TargetDevNode = filepath.Base(parts.TargetDevPath)
					break
				}
			}
		} else {
			return fmt.Errorf("No target disk found")
		}
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
	parts = Partitions{"", "", "", "", -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

	//The Sourec device which must has a recovery partition
	parts.SourceDevNode, parts.SourceDevPath, parts.Recovery_nr, err = FindPart(recoveryLabel)
	if err != nil {
		err = errors.New(fmt.Sprintf("Recovery partition (LABEL=%s) not found", recoveryLabel))
		return nil, err
	}

	err = FindTargetParts(&parts, recoveryType)
	if err != nil {
		err = errors.New(fmt.Sprintf("Target install partition not found"))
		parts = Partitions{"", "", "", "", -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
		return nil, err
	}

	//system-boot partition info
	devnode, _, sysboot_nr, err := FindPart(SysbootLabel)
	if err == nil {
		if (recoveryType != rplib.HEADLESS_INSTALLER) || (recoveryType == rplib.HEADLESS_INSTALLER && parts.SourceDevNode != devnode) {
			//Target system-boot found and must not source device in headless_installer mode
			parts.Sysboot_nr = sysboot_nr
		}
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
	cmd := exec.Command("parted", "-ms", fmt.Sprintf("/dev/%s", parts.SourceDevNode), "unit", "B", "print")
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
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

		if parts.Recovery_nr != -1 && parts.Recovery_nr == nr {
			parts.Recovery_start = start
			parts.Recovery_end = end
		} else if parts.Sysboot_nr != -1 && parts.Sysboot_nr == nr {
			parts.Sysboot_start = start
			parts.Sysboot_end = end
		} else if parts.Writable_nr != -1 && parts.Writable_nr == nr {
			parts.Writable_start = start
			parts.Writable_end = end
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
			parts.Sysboot_start = parts.Recovery_end + 1
			parts.Sysboot_end = parts.Sysboot_start + int64(partSizeMB*1024)
		}
		//TODO: To support swap partition
		// case "swap":

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
	recoverySize, err := strconv.Atoi(configs.Recovery.RecoverySize)
	if err != nil {
		return err
	}
	recoveryEnd := recoveryBegin + recoverySize

	// Build Recovery Partition
	recovery_path := fmtPartPath(parts.TargetDevPath, parts.Recovery_nr)
	rplib.Shellexec("parted", "-ms", "-a", "optimal", parts.TargetDevPath,
		"unit", "MiB",
		"mklabel", "gpt",
		"mkpart", "primary", "fat32", fmt.Sprintf("%d", recoveryBegin), fmt.Sprintf("%d", recoveryEnd),
		"name", fmt.Sprintf("%v", parts.Recovery_nr), configs.Recovery.FsLabel,
		"set", fmt.Sprintf("%v", parts.Recovery_nr), "boot", "on",
		"print")
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", configs.Recovery.FsLabel, recovery_path)

	// Copy recovery data
	err = os.MkdirAll(RECO_TAR_MNT_DIR, 0755)
	if err != nil {
		return err
	}
	err = syscall.Mount(recovery_path, RECO_TAR_MNT_DIR, "vfat", 0, "")
	if err != nil {
		return err
	}
	defer syscall.Unmount(RECO_TAR_MNT_DIR, 0)
	rplib.Shellcmd(fmt.Sprintf("cp -a %s/. %s", RECO_ROOT_DIR, RECO_TAR_MNT_DIR))
	rplib.Shellexec("sync")

	// set target grubenv to factory_restore
	cmd := exec.Command("grub-editenv", filepath.Join(RECO_TAR_MNT_DIR, "EFI/ubuntu/grubenv"), "set", "recovery_type=factory_install")
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func RestoreParts(parts *Partitions, bootloader string, partType string) error {
	var dev_path string = strings.Replace(parts.TargetDevPath, "mapper/", "", -1)
	var part_nr int
	if bootloader == "u-boot" {
		parts.Writable_nr = parts.Recovery_nr + 1 //writable is one after recovery
		part_nr = parts.Recovery_nr + 1
	} else if bootloader == "grub" {
		parts.Sysboot_nr = parts.Recovery_nr + 1
		parts.Writable_nr = parts.Sysboot_nr + 1 //writable is one after system-boot
		part_nr = parts.Recovery_nr + 1
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

	// Remove partitions after recovery
	for part_nr <= parts.Last_part_nr {
		cmd := exec.Command("parted", "-ms", dev_path, "rm", fmt.Sprintf("%v", part_nr))
		cmd.Run()
		part_nr++
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
			rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "fat32", fmt.Sprintf("%vB", parts.Sysboot_start), fmt.Sprintf("%vB", parts.Sysboot_end), "name", fmt.Sprintf("%v", parts.Sysboot_nr), SysbootLabel)
		} else if partType == "mbr" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "fat32", fmt.Sprintf("%vB", parts.Sysboot_start), fmt.Sprintf("%vB", parts.Sysboot_end))
		}
	}
	cmd := exec.Command("udevadm", "settle")
	cmd.Run()

	cmd = exec.Command("mkfs.vfat", "-F", "32", "-n", SysbootLabel, sysboot_path)
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

	// Restore writable
	_, new_end := rplib.GetPartitionBeginEnd(dev_path, parts.Sysboot_nr)
	parts.Writable_start = int64(new_end + 1)
	var writable_start string = fmt.Sprintf("%vB", parts.Writable_start)
	var writable_nr string = strconv.Itoa(parts.Writable_nr)
	writable_path := fmtPartPath(parts.TargetDevPath, parts.Writable_nr)

	if partType == "gpt" {
		rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "ext4", writable_start, "-1M", "name", writable_nr, WritableLabel)
	} else if partType == "mbr" {
		rplib.Shellexec("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "ext4", writable_start, "-1M")
	}

	cmd = exec.Command("udevadm", "settle")
	cmd.Run()

	cmd = exec.Command("mkfs.ext4", "-F", "-L", WritableLabel, writable_path)
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
