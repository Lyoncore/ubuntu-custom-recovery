package rplib

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type ConfigRecovery struct {
	Project string
	Snaps   struct {
		Kernel string
		Os     string
		Gadget string
	}
	Configs struct {
		Arch          string
		BaseImage     string
		Release       string
		PartitionType string `yaml:"partition-type"`
		Bootloader    string `yaml:"bootloader"`
		Swap          bool
		SwapFile      bool
		SwapSize      int
		BootSize      int    `yaml:"bootsize"`
		RootfsSize    int    `yaml:"rootfssize,omitempty"`
		KernelPackage string `yaml:"kernelpackage,omitempty"`
	}
	Recovery struct {
		Type                       string // one of "field_transition", "factory_install"
		RecoverySize               int
		FsLabel                    string `yaml:"filesystem-label"`
		RecoveryDevice             string `yaml:"recovery-device"`
		SystemDevice               string `yaml:"system-device"`
		InstallerFsLabel           string
		OemPreinstHookDir          string `yaml:"oem-preinst-hook-dir"`
		OemPostinstHookDir         string `yaml:"oem-postinst-hook-dir"`
		OemLogDir                  string
		SkipFactoryDiagResult      string `yaml:"skip-factory-diag-result"`
		RestoreConfirmPrehookFile  string `yaml:"restore-confirm-prehook-file"`
		RestoreConfirmPosthookFile string `yaml:"restore-confirm-posthook-file"`
		RestoreConfirmTimeoutSec   int64  `yaml:"restore-confirm-timeout"`
	}
}

func (config *ConfigRecovery) checkConfigs() (err error) {
	log.Printf("check configs ... ")

	if config.Project == "" {
		err = errors.New("'project' field not presented")
		log.Printf(err.Error())
	}

	if config.Configs.Arch == "" {
		err = errors.New("'configs -> arch' field not presented")
		log.Printf(err.Error())
	} else if config.Configs.Arch != "amd64" && config.Configs.Arch != "arm" && config.Configs.Arch != "arm64" && config.Configs.Arch != "armhf" {
		err = errors.New("'recovery -> arch' only accept \"amd64\" or \"arm\" or \"arm64\" or \"amdhf\"")
		log.Printf(err.Error())
	}

	if config.Configs.Release == "" {
		err = errors.New("'configs -> release' field not presented")
		log.Printf(err.Error())
	}

	if config.Configs.PartitionType == "" {
		err = errors.New("'recovery -> PartitionType' field not presented")
		log.Printf(err.Error())
	} else if config.Configs.PartitionType != "gpt" && config.Configs.PartitionType != "mbr" {
		err = errors.New("'recovery -> PartitionType' only accept \"gpt\" or \"mbr\"")
		log.Printf(err.Error())
	}

	if config.Configs.Bootloader == "" {
		err = errors.New("'recovery -> Bootloader' field not presented")
		log.Printf(err.Error())
	} else if config.Configs.Bootloader != "grub" && config.Configs.Bootloader != "u-boot" {
		err = errors.New("'recovery -> Bootloader' only accept \"grub\" or \"u-boot\"")
		log.Printf(err.Error())
	}

	if config.Configs.Swap != true && config.Configs.Swap != false {
		err = errors.New("'configs -> swap' field not presented")
		log.Printf(err.Error())
	}

	if config.Configs.Swap == true {
		if (config.Configs.SwapFile != true || config.Configs.SwapFile != false) && config.Configs.SwapSize <= 0 {
			err = errors.New("'configs -> swapsize' or 'configs -> swapfile' not presented")
			log.Printf(err.Error())
		}
	}

	if config.Recovery.Type == "" {
		err = errors.New("'recovery -> type' field not presented")
		log.Printf(err.Error())
	} else if config.Recovery.Type != FACTORY_RESTORE && config.Recovery.Type != HEADLESS_INSTALLER && config.Recovery.Type != FACTORY_INSTALL {
		err = errors.New(fmt.Sprintf("'recovery -> type' only accept", FACTORY_RESTORE, HEADLESS_INSTALLER, FACTORY_INSTALL))
		log.Printf(err.Error())
	}

	if config.Recovery.RecoverySize <= 0 {
		err = errors.New("'recovery -> recoverysize' must larger than 0")
		log.Printf(err.Error())
	}

	if config.Recovery.FsLabel == "" {
		err = errors.New("'recovery -> filesystem-label' field not presented")
		log.Printf(err.Error())
	}

	return err
}

func (config *ConfigRecovery) Load(configFile string) error {
	log.Printf("Loading config file %s ...", configFile)
	yamlFile, err := ioutil.ReadFile(configFile)

	if err != nil {
		return err
	}

	// Parse config file and store in configs
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return err
	}

	// Check if there is any config missing
	err = config.checkConfigs()
	return err
}

func (config *ConfigRecovery) String() string {
	io, err := yaml.Marshal(*config)
	if err != nil {
		panic(err)
	}
	return string(io)
}

// The Gadget yaml parsing from snapd/snap/gadget.go
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

type VolumeStructure struct {
	Name        string          `yaml:"name"`
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

func (gadgetInfo *GadgetInfo) Load(gadgetYaml string) error {
	log.Printf("Loading gadget.yaml %s ...", gadgetYaml)
	yamlFile, err := ioutil.ReadFile(gadgetYaml)

	if err != nil {
		return err
	}

	err = yaml.Unmarshal(yamlFile, &gadgetInfo)
	if err != nil {
		return err
	}

	return nil
}

func (gadgetInfo *GadgetInfo) GetVolumeSizebyLabel(FsLabel string) (sizeMB int, err error) {
	sizeMB = 0
	err = nil

	if gadgetInfo == nil {
		sizeMB = 0
		err = fmt.Errorf("nil gadgetInfo")
		return
	}

	// find system-boot and make copy
	for _, v := range gadgetInfo.Volumes {
		for _, st := range v.Structure {
			if st.Label == FsLabel {
				if strings.Contains(st.Size, "M") {
					sizeMB, err = strconv.Atoi(strings.Trim(st.Size, "M"))
				} else if strings.Contains(st.Size, "G") {
					if size, err := strconv.Atoi(strings.Trim(st.Size, "G")); err == nil {
						sizeMB = size * 1024
					}
				} else {
					if size, err := strconv.Atoi(string(st.Size)); err == nil {
						sizeMB = size / 1024
					}
				}
			}
		}
	}
	return
}
