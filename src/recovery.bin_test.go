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
	"os"
	"path/filepath"

	. "gopkg.in/check.v1"

	rplib "github.com/Lyoncore/ubuntu-custom-recovery/src/rplib"
)

const gptMnt = "/tmp/gptmnt"

type MainTestSuite struct{}

var _ = Suite(&MainTestSuite{})

const configName = "config.yaml"
const configSrcPath = "tests/" + configName

func (s *MainTestSuite) TestparseConfigs(c *C) {
	recoveryDir := filepath.Join("/tmp", filepath.Dir(RECO_FACTORY_DIR), "..")
	err := os.MkdirAll(recoveryDir, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(recoveryDir)

	err = rplib.FileCopy(configSrcPath, recoveryDir)
	c.Assert(err, IsNil)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	configFile := filepath.Join(recoveryDir, configName)
	parseConfigs(configFile)
}

func (s *MainTestSuite) TestpreparePartitions(c *C) {
	recoveryDir := filepath.Join("/tmp", filepath.Dir(RECO_FACTORY_DIR), "..")
	err := os.MkdirAll(recoveryDir, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(recoveryDir)

	err = rplib.FileCopy(configSrcPath, recoveryDir)
	c.Assert(err, IsNil)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"TestpreparePartitions", "recovery", "recovery"}
	configFile := filepath.Join(recoveryDir, configName)
	parseConfigs(configFile)

	// in order to test whether dir will be created
	os.RemoveAll(WRITABLE_MNT_DIR)
	os.RemoveAll(SYSBOOT_MNT_DIR)

	origGetPartitions := getPartitions
	getPartitions = func(label string, rtype string) (*Partitions, error) {
		// check if GetPartitions() is called with correct RecoveryLabel
		c.Assert(label, Equals, "recovery")
		c.Assert(rtype, Equals, rplib.FACTORY_RESTORE)
		parts := Partitions{"testSrcdevnode", "testSrcdevpath", "testTardevnode", "testTardevpath", -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
		return &parts, nil
	}
	defer func() { getPartitions = origGetPartitions }()

	origRestoreParts := restoreParts
	restoreParts = func(parts *Partitions, bootloader string, partType string, recoveryos string) error {
		// check if RestoreParts is caled correctly with *parts returned
		c.Assert(parts.SourceDevNode, Equals, "testSrcdevnode")
		c.Assert(parts.SourceDevPath, Equals, "testSrcdevpath")
		c.Assert(parts.TargetDevNode, Equals, "testTardevnode")
		c.Assert(parts.TargetDevPath, Equals, "testTardevpath")
		return nil
	}
	defer func() { restoreParts = origRestoreParts }()

	origSyscallMount := syscallMount
	var mountCalled = 0
	syscallMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		mountCalled++
		return nil
	}
	defer func() {
		// syscallMount should be called twice
		c.Assert(mountCalled, Equals, 2)
		syscallMount = origSyscallMount
	}()

	parts, _ := getPartitions("recovery", rplib.FACTORY_RESTORE)
	preparePartitions(parts)
}

func (s *MainTestSuite) TestrecoverProcess(c *C) {
	RecoveryType = "factory_restore"
	RecoveryLabel = "recovery"

	origEnableLogger := enableLogger
	enableLogger = func() error { return nil }
	defer func() { enableLogger = origEnableLogger }()

	origCopySnaps := copySnapsAsserts
	copySnapsAsserts = func() error { return nil }
	defer func() { copySnapsAsserts = origCopySnaps }()

	origAddFirstBootService := addFirstBootService
	addFirstBootService = func(recoType, recoLabel string) error {
		c.Assert(recoType, Equals, "factory_restore")
		c.Assert(recoLabel, Equals, "recovery")
		return nil
	}
	defer func() { addFirstBootService = origAddFirstBootService }()

	origRestoreAsserions := restoreAsserions
	var restoreAsserionsCalled = false
	restoreAsserions = func() error {
		restoreAsserionsCalled = true
		return nil
	}
	defer func() {
		// RestoreAsserions should be called when RecoveryType is "factory_restore"
		c.Assert(restoreAsserionsCalled, Equals, true)
		restoreAsserions = origRestoreAsserions
	}()

	origUpdateUbootEnv := updateUbootEnv
	var updateUbootEnvCalled = false
	updateUbootEnv = func(recoverylabel string) error {
		updateUbootEnvCalled = true
		return nil
	}
	defer func() {
		// The test config.yaml is u-boot sample
		c.Assert(updateUbootEnvCalled, Equals, true)
		updateUbootEnv = origUpdateUbootEnv
	}()

	origUpdateGrubCfg := updateGrubCfg
	var updateGrubCfgCalled = false
	updateGrubCfg = func(recoverylabe string, grub_cfg string, grub_env string, os string) error {
		updateGrubCfgCalled = true
		return nil
	}
	defer func() {
		// The test config.yaml is u-boot sample, it should not be called
		c.Assert(updateGrubCfgCalled, Equals, false)
		updateGrubCfg = origUpdateGrubCfg
	}()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"TestparseConfigs", "factory_restore", "recovery"}
	parseConfigs(configSrcPath)

	origGetPartitions := getPartitions
	getPartitions = func(label string, rtype string) (*Partitions, error) {
		// check if GetPartitions() is called with correct RecoveryLabel
		c.Assert(label, Equals, "recovery")
		c.Assert(rtype, Equals, rplib.FACTORY_RESTORE)
		parts := Partitions{"testSrcdevnode", "testSrcdevpath", "testTardevnode", "testTardevpath", -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
		return &parts, nil
	}
	defer func() { getPartitions = origGetPartitions }()

	parts, _ := getPartitions("recovery", rplib.FACTORY_RESTORE)
	recoverProcess(parts)
}

func (s *MainTestSuite) TestcleanupPartitions(c *C) {
	origSyscallUnMount := syscallUnMount
	var unmountCalled = 0
	syscallUnMount = func(target string, flags int) error {
		unmountCalled++
		return nil
	}
	defer func() {
		// syscallUnMount should be called twice
		c.Assert(unmountCalled, Equals, 2)
		syscallUnMount = origSyscallUnMount
	}()

	cleanupPartitions()
}
