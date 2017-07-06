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

	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"
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

	os.Args = []string{"TestparseConfigs", "factory_restor", "ESD"}
	configFile := filepath.Join(recoveryDir, configName)
	parseConfigs(configFile)

	c.Assert(RecoveryType, Equals, "factory_restor")
	c.Assert(RecoveryLabel, Equals, "ESD")
}

func (s *MainTestSuite) TestparseConfigsNoArgs(c *C) {
	recoveryDir := filepath.Join("/tmp", filepath.Dir(RECO_FACTORY_DIR), "..")
	err := os.MkdirAll(recoveryDir, 0755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(recoveryDir)

	err = rplib.FileCopy(configSrcPath, recoveryDir)
	c.Assert(err, IsNil)

	configFile := filepath.Join(recoveryDir, configName)
	// should panic with no args
	c.Assert(func() { parseConfigs(configFile) }, PanicMatches, "Need two arguments.*")
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
	getPartitions = func(label string) (*Partitions, error) {
		// check if GetPartitions() is called with correct RecoveryLabel
		c.Assert(label, Equals, "recovery")
		parts := Partitions{"testdevnode", "testdevpath", -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}
		return &parts, nil
	}
	defer func() { getPartitions = origGetPartitions }()

	origRestoreParts := restoreParts
	restoreParts = func(parts *Partitions, bootloader string, partType string) error {
		// check if RestoreParts is caled correctly with *parts returned
		c.Assert(parts.DevNode, Equals, "testdevnode")
		c.Assert(parts.DevPath, Equals, "testdevpath")
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

	preparePartitions()
}

func (s *MainTestSuite) TestrecoverProcess(c *C) {
	RecoveryType = "factory_restore"
	RecoveryLabel = "recovery"

	origEnableLogger := enableLogger
	enableLogger = func() error { return nil }
	defer func() { enableLogger = origEnableLogger }()

	origCopySnaps := copySnaps
	copySnaps = func() error { return nil }
	defer func() { copySnaps = origCopySnaps }()

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
	updateUbootEnv = func() error { return nil }
	defer func() { updateUbootEnv = origUpdateUbootEnv }()

	recoverProcess()
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
