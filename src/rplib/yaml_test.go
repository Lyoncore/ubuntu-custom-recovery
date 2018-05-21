package rplib_test

import (
	"testing"

	rplib "github.com/Lyoncore/ubuntu-recovery/src/rplib"
	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type YamlSuite struct{}

var _ = Suite(&YamlSuite{})

func (s *YamlSuite) TestLoad(c *C) {
	var configs rplib.ConfigRecovery
	err := configs.Load("test_data/config.yaml")
	c.Assert(err, IsNil)
}

func (s *YamlSuite) TestGetVolumeSizebyLabel(c *C) {
	var gi rplib.GadgetInfo
	err := gi.Load("test_data/gadget.yaml")
	c.Assert(err, IsNil)

	sizeMB, err := gi.GetVolumeSizebyLabel("system-boot")
	c.Assert(err, IsNil)
	c.Assert(sizeMB, Equals, 50)
}
