package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"

	rplib "github.com/Lyoncore/ubuntu-recovery-rplib"

	yaml "gopkg.in/yaml.v2"
)

type GadgetInfo struct {
	Volumes map[string]GadgetVolume `yaml:"volumes,omitempty"`

	// Default configuration for snaps (snap-id => key => value).
	Defaults map[string]map[string]interface{} `yaml:"defaults,omitempty"`
}

type GadgetVolume struct {
	Schema     string            `yaml:"schema"`
	Bootloader string            `yaml:"bootloader"`
	ID         string            `yaml:"id"`
	Structure  []VolumeStructure `yaml:"structure"`
}

// TODO Offsets and sizes are strings to support unit suffixes.
// Is that a good idea? *2^N or *10^N? We'll probably want a richer
// type when we actually handle these.

type VolumeStructure struct {
	Label       string          `yaml:"filesystem-label"`
	Offset      string          `yaml:"offset"`
	OffsetWrite string          `yaml:"offset-write"`
	Size        string          `yaml:"size"`
	Type        string          `yaml:"type"`
	ID          string          `yaml:"id"`
	Filesystem  string          `yaml:"filesystem"`
	Content     []VolumeContent `yaml:"content"`
}

type VolumeContent struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`

	Image       string `yaml:"image"`
	Offset      string `yaml:"offset"`
	OffsetWrite string `yaml:"offset-write"`
	Size        string `yaml:"size"`

	Unpack bool `yaml:"unpack"`
}

func main() {
	gadget_yaml_path := flag.String("gadget_yaml_path", "./gadget.yaml", "The gadget yaml path")
	gadget_unpack_path := flag.String("gadget_unpack_path", "", "The gadget unpack path")
	tmp_bootfs_path := flag.String("tmp_bootfs_path", "", "The bootfs tmp directory path")
	flag.Parse()
	var gi GadgetInfo

	if *gadget_unpack_path == "" {
		fmt.Fprintf(os.Stderr, "Error! Need gadget unpack path\n")
		os.Exit(1)
	}

	if *tmp_bootfs_path == "" {
		fmt.Fprintf(os.Stderr, "Error! Need tmp bootfs path\n")
		os.Exit(1)
	}

	gmeta, err := ioutil.ReadFile(*gadget_yaml_path)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "gadget.yaml not found:%s\n", *gadget_yaml_path)
		os.Exit(1)
	}

	if err := yaml.Unmarshal(gmeta, &gi); err != nil {
		os.Exit(1)
	}

	// find system-boot and make copy
	for _, v := range gi.Volumes {
		for _, st := range v.Structure {
			if st.Label == "system-boot" {
				for _, content := range st.Content {
					src := path.Join(*gadget_unpack_path, content.Source)
					dst := path.Join(*tmp_bootfs_path, content.Target)

					srcStat, err := os.Stat(src)
					if err != nil {
						log.Fatal(err)
						os.Exit(1)
					}

					if strings.Contains(dst, "/") == true {
						dstdir := path.Dir(dst)
						dstdirStat, err := os.Stat(dstdir)
						if err != nil {
							if err := os.MkdirAll(dstdir, 0777); err != nil {
								log.Fatal(err)
								os.Exit(1)
							}
						} else if dstdirStat.IsDir() == false {
							fmt.Fprintf(os.Stderr, "The target in bootfs should be a directory but not:%s\n", dstdir)
						}
					}

					if srcStat.IsDir() == true {
						if err := rplib.CopyTree(src, dst); err != nil {
							log.Fatal(err)
							os.Exit(1)
						}
					} else {
						if err := rplib.FileCopy(src, dst); err != nil {
							log.Fatal(err)
							os.Exit(1)
						}
					}
				}
			}
		}
	}
}
