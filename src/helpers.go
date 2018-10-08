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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	rplib "github.com/Lyoncore/ubuntu-custom-recovery/src/rplib"
)

func mib2Blocks(size int) int {
	s := size * 1024 * 1024 / 512

	return s
}

func fmtPartPath(devPath string, nr int) string {
	var partPath string
	if strings.Contains(devPath, "mmcblk") || strings.Contains(devPath, "mapper/") || strings.Contains(devPath, "nvme") || strings.Contains(devPath, "md126") {
		partPath = fmt.Sprintf("%sp%d", devPath, nr)
	} else {
		partPath = fmt.Sprintf("%s%d", devPath, nr)
	}
	return partPath
}

func usbhid() {
	log.Println("Load hid-generic and usbhid drivers for usb keyboard")

	// insert module if not exist
	cmd := exec.Command("sh", "-c", "lsmod | grep usbhid")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	if err != nil {
		rplib.Shellexec("modprobe", "usbhid")
	}

	// insert module if not exist
	cmd = exec.Command("sh", "-c", "lsmod | grep hid_generic")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err != nil {
		rplib.Shellexec("modprobe", "hid-generic")
	}
}

func GetSystemMem() (mem int64, err error) {
	fmeminfo := "/proc/meminfo"
	mem = 0
	err = nil

	FileBytes, err := ioutil.ReadFile(fmeminfo)
	if err != nil {
		err = fmt.Errorf("Read %s failed\n", fmeminfo)
		return mem, err
	}
	bufr := bytes.NewBuffer(FileBytes)
	for {
		line, err := bufr.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Sprintf("Parsing %s failed\n", fmeminfo)
		}
		ndx := strings.Index(line, "MemTotal:")
		if ndx >= 0 {
			line = strings.TrimSpace(line[9:])
			line = line[:len(line)-3]
			mem, err := strconv.ParseInt(line, 10, 64)
			if err == nil {
				return mem, err
			}
		}
	}
	err = fmt.Errorf("Read MemTotal in %s failed\n", fmeminfo)
	return mem, err
}
