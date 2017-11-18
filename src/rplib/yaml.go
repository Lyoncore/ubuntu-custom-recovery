package rplib

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"

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
		SwapSize      string
	}
	Recovery struct {
		Type                  string // one of "field_transition", "factory_install"
		RecoverySize          string
		FsLabel               string `yaml:"filesystem-label"`
		InstallerFsLabel      string
		OemPreinstHookDir     string `yaml:"oem-preinst-hook-dir"`
		OemPostinstHookDir    string `yaml:"oem-postinst-hook-dir"`
		OemLogDir             string
		SkipFactoryDiagResult string `yaml:"skip-factory-diag-result"`
	}
}

func (config *ConfigRecovery) checkConfigs() (err error) {
	log.Printf("check configs ... ")

	if config.Project == "" {
		err = errors.New("'project' field not presented")
		log.Printf(err.Error())
	}

	if config.Snaps.Kernel == "" {
		err = errors.New("'snaps -> kernel' field not presented")
		log.Printf(err.Error())
	}

	if config.Snaps.Os == "" {
		err = errors.New("'snaps -> os' field not presented")
		log.Printf(err.Error())
	}

	if config.Snaps.Gadget == "" {
		err = errors.New("'snaps -> gadget' field not presented")
		log.Printf(err.Error())
	}

	if config.Configs.Arch == "" {
		err = errors.New("'configs -> arch' field not presented")
		log.Printf(err.Error())
	} else if config.Configs.Arch != "amd64" && config.Configs.Arch != "arm" && config.Configs.Arch != "arm64" && config.Configs.Arch != "armhf" {
		err = errors.New("'recovery -> arch' only accept \"amd64\" or \"arm\" or \"arm64\" or \"amdhf\"")
		log.Printf(err.Error())
	}

	if config.Configs.BaseImage == "" {
		err = errors.New("'configs -> baseimage' field not presented")
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

	if config.Configs.SwapSize == "" {
		err = errors.New("'configs -> swapsize' field not presented")
		log.Printf(err.Error())
	}

	if config.Recovery.Type == "" {
		err = errors.New("'recovery -> type' field not presented")
		log.Printf(err.Error())
	} else if config.Recovery.Type != FACTORY_RESTORE && config.Recovery.Type != HEADLESS_INSTALLER && config.Recovery.Type != FACTORY_INSTALL {
		err = errors.New(fmt.Sprintf("'recovery -> type' only accept", FACTORY_RESTORE, HEADLESS_INSTALLER, FACTORY_INSTALL))
		log.Printf(err.Error())
	}

	if config.Recovery.RecoverySize == "" {
		err = errors.New("'recovery -> recoverysize' field not presented")
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
