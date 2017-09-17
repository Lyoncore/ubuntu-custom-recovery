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
	"fmt"
	"strings"
)

func mib2Blocks(size int) int {
	s := size * 1024 * 1024 / 512

	return s
}

func fmtPartPath(devPath string, nr int) string {
	var partPath string
	if strings.Contains(devPath, "mmcblk") || strings.Contains(devPath, "mapper/") {
		partPath = fmt.Sprintf("%sp%d", devPath, nr)
	} else {
		partPath = fmt.Sprintf("%s%d", devPath, nr)
	}
	return partPath
}