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
	"syscall"
	"testing"

	reco "github.com/Lyoncore/arm-config/src/cmd"
	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
	"github.com/snapcore/snapd/logger"
	. "gopkg.in/check.v1"
)

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

func MountTestImg(mntImg string, umntLoop string) {
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

func CreateImgs() {
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

func RmImgs() {
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
	const gptMnt = "/tmp/gptmnt"
	CreateImgs()
	defer RmImgs()

	MountTestImg(GPTimage, "")
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

	err = reco.BackupAssertions()
	c.Assert(err, IsNil)

	rdata, err := ioutil.ReadFile(fmt.Sprintf("%s/assertion", reco.ASSERTION_BACKUP_DIR))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	syscall.Unmount(gptMnt, 0)

	MountTestImg("", gptLoop)
	os.RemoveAll(reco.ASSERTION_BACKUP_DIR)
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}
