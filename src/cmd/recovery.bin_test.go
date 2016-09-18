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
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	reco "github.com/Lyoncore/arm-config/src/cmd"
	part "github.com/Lyoncore/arm-config/src/part"
	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
	"github.com/snapcore/snapd/logger"
	. "gopkg.in/check.v1"
)

const gptMnt = "/tmp/gptmnt"

func Test(t *testing.T) { TestingT(t) }

type MainTestSuite struct{}

var _ = Suite(&MainTestSuite{})

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
var part_size int64 = 600 * 1024 * 1024

func LoopUnloopImg(mntImg string, umntLoop string) {
	if umntLoop != "" {
		cmd := exec.Command("sudo", "kpartx", "-ds", fmt.Sprintf("/dev/%s", umntLoop))
		cmd.Run()
		cmd = exec.Command("sudo", "losetup", "-d", fmt.Sprintf("/dev/%s", umntLoop))
		cmd.Run()
	}

	if mntImg != "" {
		mbrLoop = rplib.Shellcmdoutput(fmt.Sprintf("sudo losetup --find --show %s | xargs basename", mntImg))
		cmd := exec.Command("sudo", "kpartx", "-avs", fmt.Sprintf("/dev/%s", mbrLoop))
		cmd.Run()
	}
}

func (s *MainTestSuite) SetUpSuite(c *C) {
	logger.SimpleSetup()
	//Create a MBR image
	mbr_img, _ := os.Create(MBRimage)
	defer mbr_img.Close()
	syscall.Fallocate(int(mbr_img.Fd()), 0, 0, part_size)

	cmd1 := exec.Command("cat", "tests/mbr.part")
	cmd2 := exec.Command("sfdisk", MBRimage)
	cmd2.Stdin, _ = cmd1.StdoutPipe()
	cmd2.Start()
	cmd1.Run()
	cmd2.Wait()

	mbrLoop = rplib.Shellcmdoutput(fmt.Sprintf("sudo losetup --find --show %s | xargs basename", MBRimage))
	cmd := exec.Command("sudo", "kpartx", "-avs", fmt.Sprintf("/dev/%s", mbrLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
	cmd.Run()
	cmd = exec.Command("sudo", "partprobe")
	cmd.Run()

	cmd = exec.Command("sudo", "kpartx", "-ds", fmt.Sprintf("/dev/%s", mbrLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "losetup", "-d", fmt.Sprintf("/dev/%s", mbrLoop))
	cmd.Run()

	//Create a GPT image
	gpt_img, _ := os.Create(GPTimage)
	defer gpt_img.Close()
	syscall.Fallocate(int(gpt_img.Fd()), 0, 0, part_size)

	cmd1 = exec.Command("cat", "tests/gpt.part")
	cmd2 = exec.Command("sfdisk", GPTimage)
	cmd2.Stdin, _ = cmd1.StdoutPipe()
	cmd2.Start()
	cmd1.Run()
	cmd2.Wait()

	gptLoop = rplib.Shellcmdoutput(fmt.Sprintf("sudo losetup --find --show %s | xargs basename", GPTimage))
	cmd = exec.Command("sudo", "kpartx", "-avs", fmt.Sprintf("/dev/%s", gptLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
	cmd.Run()
	cmd = exec.Command("sudo", "kpartx", "-ds", fmt.Sprintf("/dev/%s", mbrLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "losetup", "-d", fmt.Sprintf("/dev/%s", mbrLoop))
	cmd.Run()

}

func (s *MainTestSuite) TearDownSuite(c *C) {
	os.Remove(MBRimage)
	os.Remove(GPTimage)
}

func (s *MainTestSuite) TestConfirmRecovery(c *C) {

	in, err := ioutil.TempFile("", "")
	if err != nil {
		c.Fatal(err)
	}
	defer in.Close()

	//input 'y'
	io.WriteString(in, "y\n")
	in.Seek(0, os.SEEK_SET)
	ret_bool := reco.ConfirmRecovry(in)
	c.Check(ret_bool, Equals, true)

	//input 'Y'
	in.Seek(0, os.SEEK_SET)
	io.WriteString(in, "Y\n")
	in.Seek(0, os.SEEK_SET)
	ret_bool = reco.ConfirmRecovry(in)
	c.Check(ret_bool, Equals, true)

	//input 'n'
	in.Seek(0, os.SEEK_SET)
	io.WriteString(in, "n\n")
	in.Seek(0, os.SEEK_SET)
	ret_bool = reco.ConfirmRecovry(in)
	c.Check(ret_bool, Equals, false)

	//input 'N'
	in.Seek(0, os.SEEK_SET)
	io.WriteString(in, "N\n")
	in.Seek(0, os.SEEK_SET)
	ret_bool = reco.ConfirmRecovry(in)
	c.Check(ret_bool, Equals, false)
}

func (s *MainTestSuite) TestBackupAssertions(c *C) {
	LoopUnloopImg(GPTimage, "")
	err := os.MkdirAll(gptMnt, 0755)
	c.Assert(err, IsNil)
	defer os.Remove(gptMnt)
	err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart), gptMnt, "ext4", 0, "")
	c.Assert(err, IsNil)

	//Create testing files
	wdata := []byte("hello\n")
	err = os.MkdirAll(fmt.Sprintf("%s%s", gptMnt, reco.ASSERTION_DIR), 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(fmt.Sprintf("%s%s/assertion", gptMnt, reco.ASSERTION_DIR), wdata, 0644)
	c.Assert(err, IsNil)

	//Create symlink files
	err = os.Symlink("assertion", fmt.Sprintf("%s%s/assertion.ln", gptMnt, reco.ASSERTION_DIR))
	c.Assert(err, IsNil)
	//umount image
	syscall.Unmount(gptMnt, 0)

	// Find boot device, all other partiitons info
	parts, err := part.GetPartitions(RecoveryLabel)
	c.Assert(err, IsNil)
	err = reco.BackupAssertions(parts)
	c.Assert(err, IsNil)

	rdata, err := ioutil.ReadFile(fmt.Sprintf("%s/assertion", reco.ASSERTION_BACKUP_DIR))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	// Check link data
	rdata, err = ioutil.ReadFile(fmt.Sprintf("%s/assertion.ln", reco.ASSERTION_BACKUP_DIR))
	c.Assert(err, IsNil)
	cmp = bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	//check it's link
	stat, err := os.Lstat(fmt.Sprintf("%s/assertion.ln", reco.ASSERTION_BACKUP_DIR))
	islink := stat.Mode()&os.ModeSymlink == os.ModeSymlink
	c.Assert(islink, Equals, true)
	syscall.Unmount(gptMnt, 0)

	LoopUnloopImg("", gptLoop)
	os.RemoveAll(reco.ASSERTION_BACKUP_DIR)
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}

func (s *MainTestSuite) TestRestoreAssertions(c *C) {
	//Create testing files
	wdata := []byte("hello\n")
	err := os.MkdirAll(fmt.Sprintf("%s", reco.ASSERTION_BACKUP_DIR), 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(fmt.Sprintf("%s/assertion", reco.ASSERTION_BACKUP_DIR), wdata, 0644)
	c.Assert(err, IsNil)

	//Create symlink files
	err = os.Symlink("assertion", fmt.Sprintf("%s/assertion.ln", reco.ASSERTION_BACKUP_DIR))
	c.Assert(err, IsNil)
	defer os.RemoveAll(reco.ASSERTION_BACKUP_DIR)

	// Find boot device, all other partiitons info
	err = reco.RestoreAsserions()
	c.Assert(err, IsNil)

	// Verify
	err = os.MkdirAll(gptMnt, 0755)
	c.Assert(err, IsNil)
	defer os.Remove(gptMnt)

	rdata, err := ioutil.ReadFile(filepath.Join(reco.WRITABLE_MNT_DIR, reco.ASSERTION_DIR, "assertion"))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	// Check link data
	rdata, err = ioutil.ReadFile(filepath.Join(reco.WRITABLE_MNT_DIR, reco.ASSERTION_DIR, "assertion.ln"))
	c.Assert(err, IsNil)
	cmp = bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	//check it's link
	stat, err := os.Lstat(filepath.Join(reco.WRITABLE_MNT_DIR, reco.ASSERTION_DIR, "assertion.ln"))
	islink := stat.Mode()&os.ModeSymlink == os.ModeSymlink
	c.Assert(islink, Equals, true)

	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}

func (s *MainTestSuite) TestRestoreParts(c *C) {
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
	parts, err := part.GetPartitions(RecoveryLabel)
	c.Assert(err, IsNil)
	err = reco.RestoreParts(parts, "u-boot", "gpt")
	c.Check(err, IsNil)

	err = os.MkdirAll(gptMnt, 0755)
	c.Assert(err, IsNil)
	defer os.Remove(gptMnt)

	//Check extrat data
	err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart), gptMnt, "vfat", 0, "")
	c.Assert(err, IsNil)
	rdata, err := ioutil.ReadFile(fmt.Sprintf("%s/system-boot", gptMnt))
	c.Assert(err, IsNil)
	err = os.MkdirAll(SYS_TAR_TMP, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(SYS_TAR_TMP)
	cmd := exec.Command("tar", "--xattrs", "-xJvpf", SYS_TAR, "-C", SYS_TAR_TMP)
	cmd.Run()
	wdata, err := ioutil.ReadFile(fmt.Sprintf("%s/system-boot", SYS_TAR_TMP))
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	syscall.Unmount(gptMnt, 0)

	//Check extrat data
	err = syscall.Mount(fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart), gptMnt, "ext4", 0, "")
	c.Assert(err, IsNil)
	rdata, err = ioutil.ReadFile(fmt.Sprintf("%s/writable", gptMnt))
	c.Assert(err, IsNil)
	err = os.MkdirAll(WR_TAR_TMP, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(WR_TAR_TMP)
	cmd = exec.Command("tar", "--xattrs", "-xJvpf", WR_TAR, "-C", WR_TAR_TMP)
	cmd.Run()
	wdata, err = ioutil.ReadFile(fmt.Sprintf("%s/writable", WR_TAR_TMP))
	cmp = bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	syscall.Unmount(gptMnt, 0)

	LoopUnloopImg("", gptLoop)

	os.RemoveAll(reco.SYSBOOT_MNT_DIR)
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}
