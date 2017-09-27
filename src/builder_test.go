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

	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
	reco "github.com/Lyoncore/ubuntu-recovery/src"
	uenv "github.com/mvo5/uboot-go/uenv"
	"github.com/snapcore/snapd/logger"

	. "gopkg.in/check.v1"
)

type BuilderSuite struct{}

var _ = Suite(&BuilderSuite{})

func (s *BuilderSuite) SetUpSuite(c *C) {
	logger.SimpleSetup()
	//Create a MBR image
	rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s", MBRimage), fmt.Sprintf("bs=%d", part_size), "count=1")

	rplib.Shellexec("sgdisk", "--load-backup=tests/mbr.part", MBRimage)

	mbrLoop = rplib.Shellcmdoutput(fmt.Sprintf("losetup --find --show %s | xargs basename", MBRimage))
	rplib.Shellexec("kpartx", "-avs", filepath.Join("/dev/", mbrLoop))
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, RecoveryPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, SysbootPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", mbrLoop, WritablePart))

	rplib.Shellexec("kpartx", "-ds", filepath.Join("/dev/", mbrLoop))
	rplib.Shellexec("losetup", "-d", filepath.Join("/dev/", mbrLoop))

	//Create a GPT image
	rplib.Shellexec("dd", "if=/dev/zero", fmt.Sprintf("of=%s", GPTimage), fmt.Sprintf("bs=%d", part_size), "count=1")

	rplib.Shellexec("sgdisk", "--load-backup=tests/gpt.part", GPTimage)

	gptLoop = rplib.Shellcmdoutput(fmt.Sprintf("losetup --find --show %s | xargs basename", GPTimage))
	rplib.Shellexec("kpartx", "-avs", filepath.Join("/dev/", gptLoop))
	rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", RecoveryLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, RecoveryPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", SysbootLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, SysbootPart))
	rplib.Shellexec("mkfs.ext4", "-F", "-L", WritableLabel, fmt.Sprintf("/dev/mapper/%sp%s", gptLoop, WritablePart))
	rplib.Shellexec("kpartx", "-ds", filepath.Join("/dev/", gptLoop))
	rplib.Shellexec("losetup", "-d", filepath.Join("/dev/", gptLoop))

	cmd := exec.Command("partprobe")
	cmd.Run()
}

func (s *BuilderSuite) TearDownSuite(c *C) {
	MountTestImg("", gptLoop)
	MountTestImg("", mbrLoop)
	os.Remove(MBRimage)
	os.Remove(GPTimage)
}

func (s *BuilderSuite) TestConfirmRecovery(c *C) {

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

func (s *BuilderSuite) TestBackupAssertions(c *C) {
	MountTestImg(GPTimage, "")
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

	MountTestImg("", gptLoop)
	os.RemoveAll(reco.ASSERTION_BACKUP_DIR)
	os.RemoveAll(reco.WRITABLE_MNT_DIR)
}

func (s *BuilderSuite) TestRestoreAssertions(c *C) {
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

func (s *BuilderSuite) TestEnableLogger(c *C) {
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

func (s *BuilderSuite) TestCopySnapsAsserts(c *C) {
	//Create testing files
	const TEST_SNAP = "test.snap"
	const TEST_DEV_SNAP = "test_dev.snap"
	const TEST_ASSERT = "test.assert"

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

	err = os.MkdirAll(reco.ASSERT_PRE_SRC_PATH, 0755)
	c.Assert(err, IsNil)
	wassert := []byte("hello assert\n")
	err = ioutil.WriteFile(filepath.Join(reco.ASSERT_PRE_SRC_PATH, TEST_ASSERT), wassert, 0644)
	c.Assert(err, IsNil)

	err = reco.CopySnapsAsserts()
	c.Assert(err, IsNil)

	// Verify
	rsnap, err := ioutil.ReadFile(filepath.Join(reco.SNAPS_DST_PATH, TEST_SNAP))
	c.Assert(err, IsNil)
	cmp := bytes.Compare(rsnap, wsnap)
	c.Assert(cmp, Equals, 0)

	rdevSnap, err := ioutil.ReadFile(filepath.Join(reco.SNAPS_DST_PATH, TEST_DEV_SNAP))
	c.Assert(err, IsNil)
	cmp = bytes.Compare(rdevSnap, wdevSnap)
	c.Assert(cmp, Equals, 0)

	rassert, err := ioutil.ReadFile(filepath.Join(reco.ASSERT_DST_PATH, TEST_ASSERT))
	c.Assert(err, IsNil)
	cmp = bytes.Compare(rassert, wassert)
	c.Assert(cmp, Equals, 0)

	os.RemoveAll(reco.WRITABLE_MNT_DIR)
	os.RemoveAll("/recovery")
}

func (s *BuilderSuite) TestAddFirstBootService(c *C) {
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

	err = rplib.FileCopy("tests/writable-includes.squashfs", "/recovery/")
	c.Assert(err, IsNil)

	err = reco.AddFirstBootService(RecoveryType, RecoveryLabel)
	c.Assert(err, IsNil)

	// Verify
	// Verify /etc/systemd/system/syslog.service should be exist
	_, err = os.Stat(filepath.Join(reco.SYSTEM_DATA_PATH, "/etc/systemd/system/syslog.service"))
	c.Check(err, IsNil)

	// Verify writable-includes/system-data/etc/systemd/system/devmode-firstboot.service should be exist
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

func (s *BuilderSuite) TestUpdateUbootEnv(c *C) {
	var configs rplib.ConfigRecovery
	const CORE_SNAP = "core_9999.snap"
	const KERNEL_SNAP = "pi2-kernel_88.snap"
	//Create testing files
	err := os.MkdirAll(reco.SYSBOOT_MNT_DIR, 0755)
	c.Assert(err, IsNil)

	err = rplib.FileCopy("tests/uboot.env", reco.SYSBOOT_MNT_DIR)
	c.Assert(err, IsNil)

	err = os.MkdirAll(reco.BACKUP_SNAP_PATH, 0755)
	c.Assert(err, IsNil)

	csnap := []byte("core snap\n")
	err = ioutil.WriteFile(filepath.Join(reco.BACKUP_SNAP_PATH, CORE_SNAP), csnap, 0644)
	c.Assert(err, IsNil)

	ksnap := []byte("kernel snap\n")
	err = ioutil.WriteFile(filepath.Join(reco.BACKUP_SNAP_PATH, KERNEL_SNAP), ksnap, 0644)
	c.Assert(err, IsNil)

	err = configs.Load("tests/config.yaml")
	c.Assert(err, IsNil)

	err = reco.UpdateUbootEnv(configs.Recovery.FsLabel)
	c.Assert(err, IsNil)

	// Verify
	env, err := uenv.Open(filepath.Join(reco.SYSBOOT_MNT_DIR, "uboot.env"))
	c.Assert(err, IsNil)
	c.Check(env.Get("snap_mode"), Equals, "")
	c.Check(env.Get("recovery_type"), Equals, "factory_restore")
	c.Check(env.Get("recovery_core"), Equals, CORE_SNAP)
	c.Check(env.Get("recovery_kernel"), Equals, KERNEL_SNAP)
	c.Check(env.Get("recovery_label"), Equals, fmt.Sprintf("LABEL=%s", configs.Recovery.FsLabel))

	os.RemoveAll(reco.SYSBOOT_MNT_DIR)
}
