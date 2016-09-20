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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	reco "github.com/Lyoncore/arm-config/src"
	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
	"github.com/snapcore/snapd/logger"
	. "gopkg.in/check.v1"
)

const gptMnt = "/tmp/gptmnt"

type MainTestSuite struct{}

var _ = Suite(&MainTestSuite{})

func LoopUnloopImg(mntImg string, umntLoop string) {
	if umntLoop != "" {
		cmd := exec.Command("sudo", "kpartx", "-ds", filepath.Join("/dev/", umntLoop))
		cmd.Run()
		cmd = exec.Command("sudo", "losetup", "-d", filepath.Join("/dev/", umntLoop))
		cmd.Run()
	}

	if mntImg != "" {
		mbrLoop = rplib.Shellcmdoutput(fmt.Sprintf("sudo losetup --find --show %s | xargs basename", mntImg))
		cmd := exec.Command("sudo", "kpartx", "-avs", filepath.Join("/dev/", mbrLoop))
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
	cmd := exec.Command("sudo", "kpartx", "-avs", filepath.Join("/dev/", mbrLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))
	cmd.Run()
	cmd = exec.Command("sudo", "partprobe")
	cmd.Run()

	cmd = exec.Command("sudo", "kpartx", "-ds", filepath.Join("/dev/", mbrLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "losetup", "-d", filepath.Join("/dev/", mbrLoop))
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
	cmd = exec.Command("sudo", "kpartx", "-avs", filepath.Join("/dev/", gptLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart))
	cmd.Run()
	cmd = exec.Command("sudo", "mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
	cmd.Run()
	cmd = exec.Command("sudo", "kpartx", "-ds", filepath.Join("/dev/", mbrLoop))
	cmd.Run()
	cmd = exec.Command("sudo", "losetup", "-d", filepath.Join("/dev/", mbrLoop))
	cmd.Run()

}

func (s *MainTestSuite) TearDownSuite(c *C) {
	LoopUnloopImg("", gptLoop)
	LoopUnloopImg("", mbrLoop)
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
	err = os.MkdirAll(filepath.Join(gptMnt, reco.ASSERTION_DIR), 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(gptMnt, reco.ASSERTION_DIR, "assertion"), wdata, 0644)
	c.Assert(err, IsNil)

	//Create symlink files
	err = os.Symlink("assertion", filepath.Join(gptMnt, reco.ASSERTION_DIR, "assertion.ln"))
	c.Assert(err, IsNil)
	//umount image
	syscall.Unmount(gptMnt, 0)

	// Find boot device, all other partiitons info
	parts, err := reco.GetPartitions(RecoveryLabel)
	c.Assert(err, IsNil)
	err = reco.BackupAssertions(parts)
	c.Assert(err, IsNil)

	rdata, err := ioutil.ReadFile(filepath.Join(reco.ASSERTION_BACKUP_DIR, "assertion"))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	// Check link data
	rdata, err = ioutil.ReadFile(filepath.Join(reco.ASSERTION_BACKUP_DIR, "assertion.ln"))
	c.Assert(err, IsNil)
	cmp = bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)
	//check it's link
	stat, err := os.Lstat(filepath.Join(reco.ASSERTION_BACKUP_DIR, "assertion.ln"))
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
	err := os.MkdirAll(reco.ASSERTION_BACKUP_DIR, 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(reco.ASSERTION_BACKUP_DIR, "assertion"), wdata, 0644)
	c.Assert(err, IsNil)

	//Create symlink files
	err = os.Symlink("assertion", filepath.Join(reco.ASSERTION_BACKUP_DIR, "assertion.ln"))
	c.Assert(err, IsNil)
	defer os.RemoveAll(reco.ASSERTION_BACKUP_DIR)

	err = reco.RestoreAsserions()
	c.Assert(err, IsNil)

	// Verify
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
	cmd := exec.Command("tar", "--xattrs", "-xJvpf", SYS_TAR, "-C", SYS_TAR_TMP)
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

func (s *MainTestSuite) TestEnableLogger(c *C) {
	//Create testing files
	wdata := []byte("hello logger\n")

	err := reco.EnableLogger()
	c.Assert(err, IsNil)

	log.Printf("%s", wdata)

	// Verify
	rdata, err := ioutil.ReadFile(reco.LOG_PATH)
	c.Assert(err, IsNil)
	found := strings.Contains(string(rdata), string(wdata))
	c.Assert(found, Equals, true)

	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}

func (s *MainTestSuite) TestCopySnaps(c *C) {
	//Create testing files
	const TEST_SNAP = "test.snap"
	const TEST_DEV_SNAP = "test_dev.snap"

	err := os.MkdirAll(reco.SNAPS_SRC_PATH, 0755)
	c.Assert(err, IsNil)
	wsnap := []byte("hello snaps\n")
	err = ioutil.WriteFile(filepath.Join(reco.SNAPS_SRC_PATH, TEST_SNAP), wsnap, 0644)
	c.Assert(err, IsNil)

	err = os.MkdirAll(reco.DEV_SNAPS_SRC_PATH, 0755)
	c.Assert(err, IsNil)
	wdevSnap := []byte("hello dev snaps\n")
	err = ioutil.WriteFile(filepath.Join(reco.DEV_SNAPS_SRC_PATH, TEST_DEV_SNAP), wdevSnap, 0644)
	c.Assert(err, IsNil)

	err = reco.CopySnaps()
	c.Assert(err, IsNil)

	// Verify
	rsnap, err := ioutil.ReadFile(filepath.Join(reco.OEM_SNAPS_PATH, TEST_SNAP))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rsnap, wsnap)
	c.Assert(cmp, Equals, 0)

	rdevSnap, err := ioutil.ReadFile(filepath.Join(reco.OEM_SNAPS_PATH, TEST_DEV_SNAP))
	c.Assert(err, IsNil)
	cmp = bytes.Compare(rdevSnap, wdevSnap)
	c.Assert(cmp, Equals, 0)

	os.RemoveAll(reco.WRITABLE_MNT_DIR)
	os.RemoveAll("/recovery")
}

func (s *MainTestSuite) TestAddFirstBootService(c *C) {
	//Create testing files
	var RecoveryType = "recovery"
	var RecoveryLabel = "recovery"
	var EtcPath = "etc/systemd/system"
	err := os.MkdirAll(reco.RECO_FACTORY_DIR, 0755)
	c.Assert(err, IsNil)
	err = os.MkdirAll(reco.SYSTEM_DATA_PATH, 0755)
	c.Assert(err, IsNil)
	err = os.MkdirAll(reco.WRITABLE_MNT_DIR, 0755)
	c.Assert(err, IsNil)
	err = os.MkdirAll(EtcPath, 0755)
	c.Assert(err, IsNil)

	err = rplib.FileCopy("tests/syslog.service", EtcPath)
	c.Assert(err, IsNil)

	err = rplib.FileCopy("tests/writable_local-include.squashfs", "/recovery/")
	c.Assert(err, IsNil)

	err = reco.AddFirstBootService(RecoveryType, RecoveryLabel)
	c.Assert(err, IsNil)

	// Verify
	// Verify /etc/systemd/system/syslog.service should be exist
	_, err = os.Stat(filepath.Join(reco.SYSTEM_DATA_PATH, "/etc/systemd/system/syslog.service"))
	c.Check(err, IsNil)

	// Verify writable_local-include/system-data/etc/systemd/system/devmode-firstboot.service should be exist
	_, err = os.Stat(filepath.Join(reco.WRITABLE_MNT_DIR, "system-data/etc/systemd/system/devmode-firstboot.service"))
	c.Check(err, IsNil)

	// Verify conf.sh data
	wdata := []byte(fmt.Sprintf("RECOVERYFSLABEL=\"%s\"\nRECOVERY_TYPE=\"%s\"\n", RecoveryLabel, RecoveryType))
	rdata, err := ioutil.ReadFile(filepath.Join(reco.SYSTEM_DATA_PATH, reco.FIRSTBOOT_SREVICE_SCRIPT))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rdata, wdata)
	c.Assert(cmp, Equals, 0)

	os.RemoveAll("./etc")
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
	os.RemoveAll("/recovery")
}

func (s *MainTestSuite) TestUpdateUbootEnv(c *C) {
	//Create testing files
	err := os.MkdirAll(reco.SYSBOOT_MNT_DIR, 0755)
	c.Assert(err, IsNil)
	err = os.MkdirAll(reco.RECOVERY_PARTITION_DIR, 0755)
	c.Assert(err, IsNil)

	err = rplib.FileCopy("tests/uboot.env", reco.RECOVERY_PARTITION_DIR)
	c.Assert(err, IsNil)

	err = rplib.FileCopy("tests/uboot.env.in", reco.RECOVERY_PARTITION_DIR)
	c.Assert(err, IsNil)

	err = reco.UpdateUbootEnv()
	c.Assert(err, IsNil)

	// Verify
	// Verify SYSBOOT_MNT_DIR/uboot.env should be exist
	_, err = os.Stat(filepath.Join(reco.SYSBOOT_MNT_DIR, "uboot.env"))
	c.Check(err, IsNil)

	// Verify SYSBOOT_MNT_DIR/uboot.env.in should be exist
	_, err = os.Stat(filepath.Join(reco.SYSBOOT_MNT_DIR, "uboot.env.in"))
	c.Check(err, IsNil)

	os.RemoveAll(reco.SYSBOOT_MNT_DIR)
	os.RemoveAll(reco.RECOVERY_PARTITION_DIR)
}
