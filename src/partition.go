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
	"strconv"
	"strings"
	"syscall"

	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
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
	//DevNode: sda (W/O partiiton number),  DevPath: /dev/sda (W/O partition number)
	DevNode, DevPath                                   string
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
		} else if devPath[len(devPath)-1] == 'p' {
			part_nr := strings.Trim(fullPath, devPath)
			if partNr, err = strconv.Atoi(part_nr); err != nil {
				err = errors.New("Unknown error while FindPart")
				return "", "", -1, err
			}
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

func GetPartitions(recoveryLabel string) (*Partitions, error) {
	var err error
	const OLD_PARTITION = "/tmp/old-partition.txt"
	parts := Partitions{"", "", -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

	//Get boot device
	//The boot device must has a recovery partition
	parts.DevNode, parts.DevPath, parts.Recovery_nr, err = FindPart(recoveryLabel)
	if err != nil {
		err = errors.New(fmt.Sprintf("Recovery partition (LABEL=%s) not found", recoveryLabel))
		return nil, err
	}

	//system-boot partition info
	_, _, parts.Sysboot_nr, err = FindPart(SysbootLabel)
	if err != nil {
		//Partition not found, keep value in '-1'
		parts.Sysboot_nr = -1
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
	cmd := exec.Command("parted", "-ms", fmt.Sprintf("/dev/%s", parts.DevNode), "unit", "B", "print")
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

// TODO: bootloader if need to support grub
func RestoreParts(parts *Partitions, bootloader string, partType string) error {
	var dev_path string = strings.Replace(parts.DevPath, "mapper/", "", -1)
	if partType == "gpt" {
		rplib.Shellexec("sgdisk", dev_path, "--randomize-guids", "--move-second-header")
	} else if partType == "mbr" {
		//mbr is supported, but nothing to do
	} else {
		return fmt.Errorf("Oops, unknown partition type:%s", partType)
	}

	// Keep system-boot partition, and only mkfs
	if parts.Sysboot_nr == -1 {
		// oops, don't known the location of system-boot.
		// In the u-boot, system-boot would be in fron of recovery partition
		// If we lose system-boot, and we cannot know the proper location
		return fmt.Errorf("Oops, We lose system-boot")
	}
	sysboot_path := fmtPartPath(parts.DevPath, parts.Sysboot_nr)
	cmd := exec.Command("mkfs.vfat", "-F", "32", "-n", SysbootLabel, sysboot_path)
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
		cmd = exec.Command("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "ext4", writable_start, "-1M", "name", writable_nr, WritableLabel)
		cmd.Run()
	} else if partType == "mbr" {
		cmd = exec.Command("parted", "-a", "optimal", "-ms", dev_path, "--", "mkpart", "primary", "fat32", writable_start, "-1M")
		cmd.Run()
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
