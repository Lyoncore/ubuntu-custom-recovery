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
	. "gopkg.in/check.v1"
)

type HelpersSuite struct{}

var _ = Suite(&HelpersSuite{})

func (s *HelpersSuite) Testmib2Blocks(c *C) {
	blocks := mib2Blocks(1)

	c.Assert(blocks, Equals, 2048)
}

func (s *HelpersSuite) TestfmtPartPathMapper(c *C) {
	path := fmtPartPath("/dev/mmcblk0", 5)

	c.Assert(path, Equals, "/dev/mmcblk0p5")
}

func (s *HelpersSuite) TestfmtPartPathOther(c *C) {
	path := fmtPartPath("/dev/sdc", 3)

	c.Assert(path, Equals, "/dev/sdc3")
}
