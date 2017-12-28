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

package main_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	reco "github.com/Lyoncore/ubuntu-custom-recovery/src"
	rplib "github.com/Lyoncore/ubuntu-custom-recovery/src/rplib"
	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type GetPartSuite struct{}

var _ = Suite(&GetPartSuite{})

const (
	MBRimage           = "/tmp/mbr.img"
	GPTimage           = "/tmp/gpt.img"
	SysbootLabel       = "system-boot"
	WritableLabel      = "writable"
	RecoveryLabel      = "recovery"
	RecoveryPart       = "6"          // /dev/mapper/loopXp6
	SysbootPart        = "5"          // /dev/mapper/loopXp5
	RecoveryPart_uboot = RecoveryPart // /dev/mapper/loopXp6
	SysbootPart_uboot  = SysbootPart  // /dev/mapper/loopXp5
	RecoveryPart_grub  = "5"          // /dev/mapper/loopXp5
	SysbootPart_grub   = "6"          // /dev/mapper/loopXp6
	WritablePart       = "7"          // /dev/mapper/loopXp7
)

var mbrLoop, gptLoop string
var part_size int64 = 90 * 1024 * 1024

const gptMnt = "/tmp/gptmnt"
const mbrMnt = "/tmp/mbrmnt"

func MountTestImg(mntImg string, umntLoop string) {
	if umntLoop != "" {
		cmd := exec.Command("kpartx", "-ds", fmt.Sprintf("/dev/%s", umntLoop))
		cmd.Run()
		cmd = exec.Command("losetup", "-d", fmt.Sprintf("/dev/%s", umntLoop))
		cmd.Run()
	}
	cmd := exec.Command("udevadm", "trigger")
	cmd.Run()

	if mntImg == MBRimage {
		mbrLoop = rplib.Shellcmdoutput(fmt.Sprintf("losetup --find --show %s | xargs basename", mntImg))
		cmd := exec.Command("kpartx", "-as", fmt.Sprintf("/dev/%s", mbrLoop))
		cmd.Run()
	} else if mntImg == GPTimage {
		gptLoop = rplib.Shellcmdoutput(fmt.Sprintf("losetup --find --show %s | xargs basename", mntImg))
		cmd := exec.Command("kpartx", "-as", fmt.Sprintf("/dev/%s", gptLoop))
		cmd.Run()
	}

	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
}

func (s *GetPartSuite) SetUpTest(c *C) {
	//Create a MBR image
	rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s", MBRimage), fmt.Sprintf("bs=%d", part_size), "count=1")
	rplib.Shellexec("sgdisk", "--load-backup=tests/mbr.part", MBRimage)
	//Create a GPT image
	rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s", GPTimage), fmt.Sprintf("bs=%d", part_size), "count=1")
	rplib.Shellexec("sgdisk", "--load-backup=tests/gpt.part", GPTimage)
	cmd := exec.Command("udevadm", "trigger")
	cmd.Run()
}

func (s *GetPartSuite) TearDownTest(c *C) {
	os.Remove(MBRimage)
	os.Remove(GPTimage)
}

func getPartsConds(c *C, Label string, Loop string, passCase bool, recoCase bool, sysbootCase bool, writableCase bool) {
	parts, err := reco.GetPartitions(Label, rplib.FACTORY_RESTORE)
	if passCase == false {
		c.Assert(err, NotNil)
		c.Assert(parts, IsNil)
		return
	} else {
		c.Assert(err, IsNil)

		ret := strings.Compare(parts.SourceDevNode, Loop)
		c.Assert(ret, Equals, 0)

		ret = strings.Compare(parts.SourceDevPath, fmt.Sprintf("/dev/mapper/%s", Loop))
		c.Assert(ret, Equals, 0)

		ret = strings.Compare(parts.TargetDevNode, Loop)
		c.Assert(ret, Equals, 0)

		ret = strings.Compare(parts.TargetDevPath, fmt.Sprintf("/dev/mapper/%s", Loop))
		c.Assert(ret, Equals, 0)
	}
	if recoCase {
		nr, err := strconv.Atoi(RecoveryPart)
		c.Assert(err, IsNil)
		c.Assert(parts.Recovery_nr, Equals, nr)
	} else {
		c.Assert(parts.Recovery_nr, Equals, -1)
	}

	if sysbootCase {
		nr, err := strconv.Atoi(SysbootPart)
		c.Assert(err, IsNil)
		c.Assert(parts.Sysboot_nr, Equals, nr)
	} else {
		c.Assert(parts.Sysboot_nr, Equals, -1)
	}

	if writableCase {
		nr, err := strconv.Atoi(WritablePart)
		c.Assert(err, IsNil)
		c.Assert(parts.Writable_nr, Equals, nr)
	} else {
		c.Assert(parts.Writable_nr, Equals, -1)
	}

	c.Assert(parts.Last_part_nr, Equals, 8)
}

func _build_image(image string) {
	cmd := exec.Command("kpartx", "-ds", image)
	cmd.Run()

	rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s", image), fmt.Sprintf("bs=%d", part_size), "count=1")
	rplib.Shellexec("sgdisk", "--load-backup=tests/mbr.part", image)
}

func _clear_partition() {
	for count := 0; count < 10; count++ {
		parts, err := reco.GetPartitions(RecoveryLabel, rplib.FACTORY_RESTORE)
		if err != nil {
			fmt.Println("Partitions are cleared, continoue")
			break
		}

		fmt.Println(parts)
		cmd := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s", parts.SourceDevPath))
		cmd.Run()
		cmd = exec.Command(fmt.Sprintf("partparobe %s", parts.SourceDevPath))
		cmd.Run()
		cmd = exec.Command("udevadm", "trigger")
		cmd.Run()

		fmt.Println("wait partition be cleared")
		time.Sleep(1 * time.Second)
	}
}

func (s *GetPartSuite) TestGetPartitions(c *C) {
	_clear_partition()
	_build_image(MBRimage)
	//Case in MBR
	MountTestImg(MBRimage, "")

	//Case 1, all, not exist
	getPartsConds(c, RecoveryLabel, mbrLoop, false, false, false, false)

	//Case 2, only recovery partition exist
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
	cmd := exec.Command("udevadm", "trigger")
	cmd.Run()
	getPartsConds(c, RecoveryLabel, mbrLoop, true, true, false, false)

	//Case 3, only recovery, system-boot partition exist
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
	getPartsConds(c, RecoveryLabel, mbrLoop, true, true, true, false)

	//Case4 : recovery, writable, system-boot exist
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
	getPartsConds(c, RecoveryLabel, mbrLoop, true, true, true, true)

	//GPT case
	//Clear old data in image file
	MountTestImg("", mbrLoop)
	_clear_partition()
	_build_image(GPTimage)
	MountTestImg(GPTimage, "")

	//Case 1, all, not exist
	getPartsConds(c, RecoveryLabel, gptLoop, false, false, false, false)

	//Case 2, only recovery partition exist
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart))
	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
	getPartsConds(c, RecoveryLabel, gptLoop, true, true, false, false)

	//Case 3, only recovery, system-boot partition exist
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart))
	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
	getPartsConds(c, RecoveryLabel, gptLoop, true, true, true, false)

	//Case4 : recovery, writable, system-boot exist
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
	getPartsConds(c, RecoveryLabel, gptLoop, true, true, true, true)

	MountTestImg("", gptLoop)
}

func (s *GetPartSuite) TestFindPart(c *C) {

	MountTestImg(MBRimage, "")
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
	cmd := exec.Command("udevadm", "trigger")
	cmd.Run()

	SourceDevNode, SourceDevPath, PartNr, err := reco.FindPart(RecoveryLabel)
	c.Check(err, IsNil)
	c.Check(SourceDevNode, Equals, mbrLoop)
	c.Check(SourceDevPath, Equals, fmt.Sprintf("/dev/mapper/%s", SourceDevNode))
	nr, err := strconv.Atoi(RecoveryPart)
	c.Check(err, IsNil)
	c.Check(PartNr, Equals, nr)

	MountTestImg(GPTimage, mbrLoop)
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
	cmd = exec.Command("udevadm", "trigger")
	cmd.Run()
	SourceDevNode, SourceDevPath, PartNr, err = reco.FindPart(RecoveryLabel)
	c.Check(err, IsNil)
	c.Check(SourceDevNode, Equals, mbrLoop)
	c.Check(SourceDevPath, Equals, fmt.Sprintf("/dev/mapper/%s", SourceDevNode))
	nr, err = strconv.Atoi(RecoveryPart)
	c.Check(err, IsNil)
	c.Check(PartNr, Equals, nr)
	MountTestImg("", mbrLoop)

	SourceDevNode, SourceDevPath, PartNr, err = reco.FindPart("WrongLabel")
	c.Check(err, NotNil)
	c.Check(SourceDevNode, Equals, "")
	c.Check(SourceDevPath, Equals, "")
	c.Check(PartNr, Equals, -1)
}

func (s *GetPartSuite) TestRestoreParts(c *C) {
	const (
		SYS_TAR     = "tests/system-boot.tar.xz"
		WR_TAR      = "tests/writable.tar.xz"
		RECO_PATH   = "/recovery/"
		TAR_PATH    = RECO_PATH + "factory/"
		SYS_TAR_TMP = "/tmp/systar"
		WR_TAR_TMP  = "/tmp/wrtar"
	)

	os.MkdirAll(TAR_PATH, 0755)
	rplib.FileCopy(SYS_TAR, TAR_PATH)
	rplib.FileCopy(WR_TAR, TAR_PATH)
	defer os.RemoveAll(RECO_PATH)

	/*
		// u-boot, mbr case
		// Find boot device, all other partiitons info
		MountTestImg(MBRimage, "")
		rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
		rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
		rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
		cmd := exec.Command("udevadm", "trigger")
		cmd.Run()
		parts, err := reco.GetPartitions(RecoveryLabel, rplib.FACTORY_RESTORE)
		c.Assert(err, IsNil)
		err = reco.RestoreParts(parts, "u-boot", "mbr")
		c.Check(err, IsNil)

		err = os.MkdirAll(mbrMnt, 0755)
		c.Assert(err, IsNil)
		defer os.Remove(mbrMnt)

		//Check extrat data
		err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart), mbrMnt, "vfat", 0, "")
		c.Assert(err, IsNil)
		rdata, err := ioutil.ReadFile(filepath.Join(mbrMnt, "system-boot"))
		c.Assert(err, IsNil)
		err = os.MkdirAll(SYS_TAR_TMP, 0755)
		c.Assert(err, IsNil)
		defer os.RemoveAll(SYS_TAR_TMP)
		cmd = exec.Command("tar", "--xattrs", "-xJvpf", SYS_TAR, "-C", SYS_TAR_TMP)
		cmd.Run()
		wdata, err := ioutil.ReadFile(filepath.Join(SYS_TAR_TMP, "system-boot"))
		cmp := bytes.Compare(rdata, wdata)
		c.Assert(cmp, Equals, 0)
		syscall.Unmount(mbrMnt, 0)

		//Check extrat data
		err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart), mbrMnt, "ext4", 0, "")
		c.Assert(err, IsNil)
		rdata, err = ioutil.ReadFile(filepath.Join(mbrMnt, "writable"))
		c.Assert(err, IsNil)
		err = os.MkdirAll(WR_TAR_TMP, 0755)
		c.Assert(err, IsNil)
		defer os.RemoveAll(WR_TAR_TMP)
		cmd = exec.Command("tar", "--xattrs", "-xJvpf", WR_TAR, "-C", WR_TAR_TMP)
		cmd.Run()
		wdata, err = ioutil.ReadFile(filepath.Join(WR_TAR_TMP, "writable"))
		cmp = bytes.Compare(rdata, wdata)
		c.Assert(cmp, Equals, 0)
		syscall.Unmount(mbrMnt, 0)
	*/
	// grub, gpt case
	// Find boot device, all other partiitons info
	MountTestImg(GPTimage, mbrLoop)
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart_grub))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart_grub))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
	cmd := exec.Command("udevadm", "trigger")
	cmd.Run()
	parts, err := reco.GetPartitions(RecoveryLabel, rplib.FACTORY_RESTORE)
	c.Assert(err, IsNil)
	err = reco.RestoreParts(parts, "grub", "gpt")
	c.Check(err, IsNil)

	err = os.MkdirAll(gptMnt, 0755)
	c.Assert(err, IsNil)
	defer os.Remove(gptMnt)

	//Check extrat data
	err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart_grub), gptMnt, "vfat", 0, "")
	c.Assert(err, IsNil)
	rdata, err := ioutil.ReadFile(filepath.Join(gptMnt, "system-boot"))
	c.Assert(err, IsNil)
	err = os.MkdirAll(SYS_TAR_TMP, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(SYS_TAR_TMP)
	cmd = exec.Command("tar", "--xattrs", "-xJvpf", SYS_TAR, "-C", SYS_TAR_TMP)
	cmd.Run()
	wdata, err := ioutil.ReadFile(filepath.Join(SYS_TAR_TMP, "system-boot"))
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	syscall.Unmount(gptMnt, 0)

	//Check extrat data
	err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart), gptMnt, "ext4", 0, "")
	c.Assert(err, IsNil)
	rdata, err = ioutil.ReadFile(filepath.Join(gptMnt, "writable"))
	c.Assert(err, IsNil)
	err = os.MkdirAll(WR_TAR_TMP, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(WR_TAR_TMP)
	cmd = exec.Command("tar", "--xattrs", "-xJvpf", WR_TAR, "-C", WR_TAR_TMP)
	cmd.Run()
	wdata, err = ioutil.ReadFile(filepath.Join(WR_TAR_TMP, "writable"))
	cmp = bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	syscall.Unmount(gptMnt, 0)

	MountTestImg("", gptLoop)

	//Unsupported partition type
	err = reco.RestoreParts(parts, "u-boot", "OthersPartType")
	c.Check(err.Error(), Equals, "Oops, unknown partition type:OthersPartType")

	//Unsupported partition type
	err = reco.RestoreParts(parts, "OthersBootloader", "gpt")
	c.Check(err.Error(), Equals, "Oops, unknown bootloader:OthersBootloader")

	os.RemoveAll(reco.SYSBOOT_MNT_DIR)
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}
