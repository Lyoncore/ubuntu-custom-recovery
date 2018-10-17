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
	"math"
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

func GetSystemMemkB() (mem int64, err error) {
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

func CalcSwapFileSizeMB() (size int64, err error) {
	mem_size, err := GetSystemMemkB()
	if err != nil {
		return 0, err
	}

	mem_sizeMB := math.Round(float64(mem_size) / 1000)
	sizef := math.Round(mem_sizeMB + math.Sqrt(mem_sizeMB))
	size = int64(sizef)

	return size, err
}

func GetSwapFileOffset(swapFile string) (int, error) {
	swaptmp := "/tmp/swap.txt"

	if _, err := os.Stat(swapFile); os.IsNotExist(err) {
		return 0, err
	}
	tmp, _ := os.Create(swaptmp)
	c1 := exec.Command("filefrag", "-v", swapFile)
	c2 := exec.Command("grep", " 0:")
	c3 := exec.Command("cut", "-d", ":", "-f", "3")
	c4 := exec.Command("cut", "-d", ".", "-f", "1")
	c2.Stdin, _ = c1.StdoutPipe()
	c3.Stdin, _ = c2.StdoutPipe()
	c4.Stdin, _ = c3.StdoutPipe()
	c4.Stdout = tmp
	_ = c4.Start()
	_ = c3.Start()
	_ = c2.Start()
	_ = c1.Run()
	_ = c2.Wait()
	_ = c3.Wait()
	_ = c4.Wait()

	out, _ := ioutil.ReadFile(swaptmp)
	i, err := strconv.Atoi(strings.TrimSpace(string(out)))
	return i, err
}
