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

package part

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

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
	Recovery_nr, Sysboot_nr, Writable_nr int
	Recovery_start, Recovery_end         int64
	Sysboot_start, Sysboot_end           int64
	Writable_start, Writable_end         int64
}

const (
	SysbootLabel  = "system-boot"
	WritableLabel = "writable"
)

func GetPartitions(device string, recoveryLabel string) (*Partitions, error) {
	var err error
	const OLD_PARTITION = "/tmp/old-partition.txt"
	parts := Partitions{-1, -1, -1, -1, -1, -1, -1, -1, -1}

	//recovery partiiton info
	recovery_part := rplib.Findfs(fmt.Sprintf("LABEL=%s", recoveryLabel))
	part_nr := recovery_part[len(recovery_part)-1:]
	parts.Recovery_nr, err = strconv.Atoi(part_nr)
	if err != nil {
		return nil, err
	}

	//system-boot partition info
	sysboot_part := rplib.Findfs(fmt.Sprintf("LABEL=%s", SysbootLabel))
	part_nr = sysboot_part[len(sysboot_part)-1:]
	parts.Sysboot_nr, err = strconv.Atoi(part_nr)
	if err != nil {
		return nil, err
	}

	//writable-boot partition info
	writable_part := rplib.Findfs(fmt.Sprintf("LABEL=%s", WritableLabel))
	part_nr = writable_part[len(writable_part)-1:]
	parts.Writable_nr, err = strconv.Atoi(part_nr)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("parted", "-ms", device, "unit", "B", "print")
	stdout, _ := cmd.StdoutPipe()
	scanner := bufio.NewScanner(stdout)
	cmd.Start()
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

		if parts.Recovery_nr == nr {
			parts.Recovery_start = start
			parts.Recovery_end = end
		} else if parts.Sysboot_nr == nr {
			parts.Sysboot_start = start
			parts.Sysboot_end = end
		} else if parts.Writable_nr == nr {
			parts.Writable_start = start
			parts.Writable_end = end
		}
	}
	cmd.Wait()
	err = scanner.Err()
	if err != nil {
		return nil, err
	}

	return &parts, nil
}
