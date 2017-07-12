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

	reco "github.com/Lyoncore/arm-config/src"
	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
	"github.com/snapcore/snapd/logger"
	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type GetPartSuite struct{}

var _ = Suite(&GetPartSuite{})

const (
	MBRimage      = "/tmp/mbr.img"
	GPTimage      = "/tmp/gpt.img"
	SysbootLabel  = "system-boot"
	WritableLabel = "writable"
	RecoveryLabel = "recovery"
	RecoveryPart  = "6" // /dev/mapper/loopXp6
	SysbootPart   = "5" // /dev/mapper/loopXp5
	WritablePart  = "7" // /dev/mapper/loopXp7
)

var mbrLoop, gptLoop string
var part_size int64 = 90 * 1024 *1024

func MountTestImg(mntImg string, umntLoop string) {
	if umntLoop != "" {
		cmd := exec.Command("kpartx", "-ds", fmt.Sprintf("/dev/%s", umntLoop))
		cmd.Run()
		cmd = exec.Command("losetup", "-d", fmt.Sprintf("/dev/%s", umntLoop))
		cmd.Run()
	}

	if mntImg == MBRimage {
		mbrLoop = rplib.Shellcmdoutput(fmt.Sprintf("losetup --find --show %s | xargs basename", mntImg))
		cmd := exec.Command("kpartx", "-avs", fmt.Sprintf("/dev/%s", mbrLoop))
		cmd.Run()
	} else {
		gptLoop = rplib.Shellcmdoutput(fmt.Sprintf("losetup --find --show %s | xargs basename", mntImg))
		cmd := exec.Command("kpartx", "-avs", fmt.Sprintf("/dev/%s", gptLoop))
		cmd.Run()
	}

	cmd := exec.Command("partprobe")
        cmd.Run()
}

func (s *GetPartSuite) SetUpTest(c *C) {
        logger.SimpleSetup()
        //Create a MBR image
        rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s",MBRimage), fmt.Sprintf("bs=%d",part_size), "count=1")
        rplib.Shellexec("sgdisk", "--load-backup=tests/mbr.part", MBRimage)

        //Create a GPT image
        rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s",GPTimage), fmt.Sprintf("bs=%d",part_size), "count=1")
        rplib.Shellexec("sgdisk", "--load-backup=tests/gpt.part", GPTimage)

	cmd := exec.Command("partprobe")
        cmd.Run()
}

func (s *GetPartSuite) TearDownTest(c *C) {
	//os.Remove(MBRimage)
	//os.Remove(GPTimage)
}

func getPartsConds(c *C, Label string, Loop string, passCase bool, recoCase bool, sysbootCase bool, writableCase bool) {
	parts, err := reco.GetPartitions(Label)
	if passCase == false {
		c.Check(err, NotNil)
		c.Check(parts, IsNil)
		return
	} else {
		c.Check(err, IsNil)

		ret := strings.Compare(parts.DevNode, Loop)
		c.Check(ret, Equals, 0)

		ret = strings.Compare(parts.DevPath, fmt.Sprintf("/dev/mapper/%s", Loop))
		c.Check(ret, Equals, 0)
	}

	if recoCase {
		nr, err := strconv.Atoi(RecoveryPart)
		c.Check(err, IsNil)
		c.Check(parts.Recovery_nr, Equals, nr)
	} else {
		c.Check(parts.Recovery_nr, Equals, -1)
	}

	if sysbootCase {
		nr, err := strconv.Atoi(SysbootPart)
		c.Check(err, IsNil)
		c.Check(parts.Sysboot_nr, Equals, nr)
	} else {
		c.Check(parts.Sysboot_nr, Equals, -1)
	}

	if writableCase {
		nr, err := strconv.Atoi(WritablePart)
		c.Check(err, IsNil)
		c.Check(parts.Writable_nr, Equals, nr)
	} else {
		c.Check(parts.Writable_nr, Equals, -1)
	}

	c.Check(parts.Last_part_nr, Equals, 8)
}

func (s *GetPartSuite) TestgetPartitions(c *C) {

	//Case in MBR
	MountTestImg(MBRimage, "")

	//Case 1, all, not exist
	getPartsConds(c, RecoveryLabel, mbrLoop, false, false, false, false)
	
	//Case 2, only recovery partition exist
        rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
        cmd := exec.Command("partprobe")
        cmd.Run()
	getPartsConds(c, RecoveryLabel, mbrLoop, true, true, false, false)

	//Case 3, only recovery, system-boot partition exist
        rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
        cmd = exec.Command("partprobe")
        cmd.Run()
	getPartsConds(c, RecoveryLabel, mbrLoop, true, true, true, false)

	//Case4 : recovery, writable, system-boot exist
        rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
        cmd = exec.Command("partprobe")
        cmd.Run()
	getPartsConds(c, RecoveryLabel, mbrLoop, true, true, true, true)

	//GPT case
	MountTestImg(GPTimage, mbrLoop)

	//Case 1, all, not exist
	getPartsConds(c, RecoveryLabel, gptLoop, false, false, false, false)

	//Case 2, only recovery partition exist
        rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart))
        cmd = exec.Command("partprobe")
        cmd.Run()
	getPartsConds(c, RecoveryLabel, gptLoop, true, true, false, false)

	//Case 3, only recovery, system-boot partition exist
        rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart))
        cmd = exec.Command("partprobe")
        cmd.Run()
	getPartsConds(c, RecoveryLabel, gptLoop, true, true, true, false)

	//Case4 : recovery, writable, system-boot exist
        rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
        cmd = exec.Command("partprobe")
        cmd.Run()
	getPartsConds(c, RecoveryLabel, gptLoop, true, true, true, true)

	MountTestImg("", gptLoop)
}

func (s *GetPartSuite) TestFindPart(c *C) {

	MountTestImg(MBRimage, "")
        rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
        rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
        rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
        cmd := exec.Command("partprobe")
        cmd.Run()

	DevNode, DevPath, PartNr, err := reco.FindPart(RecoveryLabel)
	c.Check(err, IsNil)
	c.Check(DevNode, Equals, mbrLoop)
	c.Check(DevPath, Equals, fmt.Sprintf("/dev/mapper/%s", DevNode))
	nr, err := strconv.Atoi(RecoveryPart)
	c.Check(err, IsNil)
	c.Check(PartNr, Equals, nr)

	MountTestImg(GPTimage, mbrLoop)
        rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
        rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
        rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
        cmd = exec.Command("partprobe")
        cmd.Run()
	DevNode, DevPath, PartNr, err = reco.FindPart(RecoveryLabel)
	c.Check(err, IsNil)
	c.Check(DevNode, Equals, mbrLoop)
	c.Check(DevPath, Equals, fmt.Sprintf("/dev/mapper/%s", DevNode))
	nr, err = strconv.Atoi(RecoveryPart)
	c.Check(err, IsNil)
	c.Check(PartNr, Equals, nr)
	MountTestImg("", mbrLoop)

	DevNode, DevPath, PartNr, err = reco.FindPart("WrongLabel")
	c.Check(err, NotNil)
	c.Check(DevNode, Equals, "")
	c.Check(DevPath, Equals, "")
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

	// GPT case
	// Find boot device, all other partiitons info
	LoopUnloopImg(GPTimage, "")
        rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart))
        rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart))
        rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
        cmd := exec.Command("partprobe")
        cmd.Run()
	parts, err := reco.GetPartitions(RecoveryLabel)
	c.Assert(err, IsNil)
	err = reco.RestoreParts(parts, "u-boot", "gpt")
	c.Check(err, IsNil)

	err = os.MkdirAll(gptMnt, 0755)
	c.Assert(err, IsNil)
	defer os.Remove(gptMnt)

	//Check extrat data
	err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart), gptMnt, "vfat", 0, "")
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

	LoopUnloopImg("", gptLoop)

	os.RemoveAll(reco.SYSBOOT_MNT_DIR)
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}
