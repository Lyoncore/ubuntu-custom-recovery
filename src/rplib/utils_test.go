package rplib_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"

	. "gopkg.in/check.v1"
)

type UtilsSuite struct {
	srcdir string
	dstdir string
}

var _ = Suite(&UtilsSuite{})

//make temp dir with contents:
// tmp/file1
// tmp/dir1
// tmp/dir1/file2
// tmp/dir1/link1 -> file2
// tmp/dir1/dir2/file3
const (
	FILE1 = "file1"
	DIR1  = "dir1/"
	FILE2 = "dir1/file2"
	LINK1 = "dir1/link1" //file2
	DIR2  = "dir1/dir2"
	FILE3 = "dir1/dir2/file3"
)

func (s *UtilsSuite) SetUpSuite(c *C) {
	s.srcdir = c.MkDir()
	s.dstdir = c.MkDir()

	os.Mkdir(fmt.Sprintf("%s/%s", s.srcdir, DIR1), 0755) // tmp/xxx/dir1
	os.Mkdir(fmt.Sprintf("%s/%s", s.srcdir, DIR2), 0755) // tmp/xxx/dir1/dir2

	file1 := fmt.Sprintf("%s/%s", s.srcdir, FILE1)
	d_f1 := []byte(file1)
	ioutil.WriteFile(file1, d_f1, 0644) // tmp/xxx/file1

	file2 := fmt.Sprintf("%s/%s", s.srcdir, FILE2)
	d_f2 := []byte(file2)
	ioutil.WriteFile(file2, d_f2, 0644) // tmp/xxx/dir1/file2

	file3 := fmt.Sprintf("%s/%s", s.srcdir, FILE3)
	d_f3 := []byte(file3)
	ioutil.WriteFile(file3, d_f3, 0644) // tmp/xxx/dir1/file3

	os.Symlink(fmt.Sprintf("%s/%s", s.srcdir, FILE2), fmt.Sprintf("%s/%s", s.srcdir, LINK1)) // tmp/dir1/link1
}

func (s *UtilsSuite) TestCopyTree(c *C) {
	err := rplib.CopyTree(s.srcdir, s.dstdir)
	c.Assert(err, IsNil)

	//Check file1 data
	srcdata, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", s.srcdir, FILE1))
	c.Assert(err, IsNil)
	dstdata, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", s.dstdir, FILE1))
	cmp := bytes.Compare(srcdata, dstdata)
	c.Assert(cmp, Equals, 0)

	//Check file2 data
	srcdata, err = ioutil.ReadFile(fmt.Sprintf("%s/%s", s.srcdir, FILE2))
	c.Assert(err, IsNil)
	dstdata, err = ioutil.ReadFile(fmt.Sprintf("%s/%s", s.dstdir, FILE2))
	cmp = bytes.Compare(srcdata, dstdata)
	c.Assert(cmp, Equals, 0)

	//Check file3 data
	srcdata, err = ioutil.ReadFile(fmt.Sprintf("%s/%s", s.srcdir, FILE3))
	c.Assert(err, IsNil)
	dstdata, err = ioutil.ReadFile(fmt.Sprintf("%s/%s", s.dstdir, FILE3))
	cmp = bytes.Compare(srcdata, dstdata)
	c.Assert(cmp, Equals, 0)

	//Check link3 data
	srcdata, err = ioutil.ReadFile(fmt.Sprintf("%s/%s", s.srcdir, LINK1))
	c.Assert(err, IsNil)
	dstdata, err = ioutil.ReadFile(fmt.Sprintf("%s/%s", s.dstdir, LINK1))
	cmp = bytes.Compare(srcdata, dstdata)
	c.Assert(cmp, Equals, 0)
}
