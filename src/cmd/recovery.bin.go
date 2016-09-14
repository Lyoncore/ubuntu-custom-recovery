package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvo5/uboot-go/uenv"
)

import rplib "github.com/Lyoncore/ubuntu-recovery-rplib"

var version string
var commit string
var commitstamp string
var build_date string

// NOTE: this is hardcoded in `devmode-firstboot.sh`; keep in sync
const DISABLE_CLOUD_OPTION = ""

func getRecoveryPartition(device string, RECOVERY_LABEL string) (recovery_nr int, recovery_end int64) {
	var err error
	const OLD_PARTITION = "/tmp/old-partition.txt"
	recovery_nr = -1

	recovery_part := rplib.Findfs(fmt.Sprintf("LABEL=%s", RECOVERY_LABEL))
	recovery_nr, err = strconv.Atoi(strings.Trim(strings.Trim(recovery_part, device), "p"))
	rplib.Shellcmd(fmt.Sprintf("parted -ms %s unit B print | sed -n '1,2!p' > %s", device, OLD_PARTITION))

	// Read information of recovery partition
	// Keep only recovery partition
	var f *(os.File)
	f, err = os.Open(OLD_PARTITION)
	rplib.Checkerr(err)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		log.Println("line: ", line)
		fields := strings.Split(line, ":")
		log.Println("fields: ", fields)
		nr, err := strconv.Atoi(fields[0])
		rplib.Checkerr(err)
		if recovery_nr == nr {
			end, err := strconv.ParseInt(strings.TrimRight(fields[2], "B"), 10, 64)
			rplib.Checkerr(err)
			recovery_end = end
			log.Println("recovery_nr:", recovery_nr)
			log.Println("recovery_end:", recovery_end)
			continue
		}

		// delete all the partitions after recovery partition
		rplib.Shellexec("parted", "-ms", device, "rm", fmt.Sprintf("%v", nr))
	}
	err = scanner.Err()
	rplib.Checkerr(err)

	return
}

func mib2Blocks(size int) int {
	s := size * 1024 * 1024 / 512

	if s%4 != 0 {
		panic(fmt.Sprintf("invalid partition size: %d", s))
	}

	return s
}

func recreateRecoveryPartition(device string, RECOVERY_LABEL string, recovery_nr int, recovery_end int64) (normal_boot_nr int) {
	last_end := recovery_end
	nr := recovery_nr + 1

	// Read information of recovery partition
	// Keep only recovery partition

	partitions := [...]string{"system-boot", "writable"}
	for _, partition := range partitions {
		log.Println("last_end:", last_end)
		log.Println("nr:", nr)

		var end_size string
		var fstype string
		var block string
		// TODO: allocate partition according to gadget.PartitionLayout
		// create the new partition
		if "writable" == partition {
			end_size = "-1M"
			fstype = "ext4"
		} else if "system-boot" == partition {
			size := 64 * 1024 * 1024 // 64 MB in Bytes
			end_size = strconv.FormatInt(last_end+int64(size), 10) + "B"
			fstype = "fat32"
		}
		log.Println("end_size:", end_size)

		// make partition with optimal alignment
		if configs.Configs.Bootloader == "gpt" {
			rplib.Shellexec("parted", "-a", "optimal", "-ms", device, "--", "mkpart", "primary", fstype, fmt.Sprintf("%vB", last_end+1), end_size, "name", fmt.Sprintf("%v", nr), partition)
		} else { //mbr don't support partition name
			rplib.Shellexec("parted", "-a", "optimal", "-ms", device, "--", "mkpart", "primary", fstype, fmt.Sprintf("%vB", last_end+1), end_size)
		}
		_, new_end := rplib.GetPartitionBeginEnd64(device, nr)

		if "system-boot" == partition {
			normal_boot_nr = nr
			log.Println("normal_boot_nr:", normal_boot_nr)
		}

		rplib.Shellexec("udevadm", "settle")
		rplib.Shellexec("parted", "-ms", device, "unit", "B", "print") // debug

		if strings.Contains(device, "mmcblk") == true {
			block = fmt.Sprintf("%sp%d", device, nr) //mmcblk0pX
		} else {
			block = fmt.Sprintf("%s%d", device, nr)
		}
		log.Println("block:", block)
		// create filesystem and dump snap system
		switch partition {
		case "writable":
			rplib.Shellexec("mkfs.ext4", "-F", "-L", "writable", block)
			err := os.MkdirAll("/tmp/writable/", 0755)
			rplib.Checkerr(err)
			err = syscall.Mount(block, "/tmp/writable", "ext4", 0, "")
			rplib.Checkerr(err)
			defer syscall.Unmount("/tmp/writable", 0)
			rplib.Shellexec("tar", "--xattrs", "-xJvpf", "/recovery/factory/writable.tar.xz", "-C", "/tmp/writable/")
		case "system-boot":
			rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", "system-boot", block)
			err := os.MkdirAll("/tmp/system-boot/", 0755)
			rplib.Checkerr(err)
			err = syscall.Mount(block, "/tmp/system-boot", "vfat", 0, "")
			rplib.Checkerr(err)
			defer syscall.Unmount("/tmp/system-boot", 0)
			rplib.Shellexec("tar", "--xattrs", "-xJvpf", "/recovery/factory/system-boot.tar.xz", "-C", "/tmp/system-boot/")
			rplib.Shellexec("parted", "-ms", device, "set", strconv.Itoa(nr), "boot", "on")
		}
		last_end = new_end
		nr = nr + 1
	}
	return
}

func hack_grub_cfg(recovery_type_cfg string, recovery_type_label string, recovery_part_label string, grub_cfg string) {
	// add cloud-init disabled option
	// sed -i "s/^set cmdline="\(.*\)"$/set cmdline="\1 $cloud_init_disabled"/g"
	rplib.Shellexec("sed", "-i", "s/^set cmdline=\"\\(.*\\)\"$/set cmdline=\"\\1 $cloud_init_disabled\"/g", grub_cfg)

	// add recovery grub menuentry
	f, err := os.OpenFile(grub_cfg, os.O_APPEND|os.O_WRONLY, 0600)
	rplib.Checkerr(err)

	text := fmt.Sprintf(`
menuentry "%s" {
        # load recovery system
        echo "[grub.cfg] load %s system"
        search --no-floppy --set --label "%s"
        echo "[grub.cfg] root: ${root}"
        set cmdline="root=LABEL=%s ro init=/lib/systemd/systemd console=ttyS0 console=tty1 panic=-1 -- recoverytype=%s"
        echo "[grub.cfg] loading kernel..."
        loopback loop0 /kernel.snap
        linux (loop0)/vmlinuz $cmdline
        echo "[grub.cfg] loading initrd..."
        initrd /initrd.img
        echo "[grub.cfg] boot..."
        boot
}`, recovery_type_label, recovery_type_cfg, recovery_part_label, recovery_part_label, recovery_type_cfg)
	if _, err = f.WriteString(text); err != nil {
		panic(err)
	}

	f.Close()
}

func hack_uboot_env(uboot_cfg string) {
	env, err := uenv.Open(uboot_cfg)
	rplib.Checkerr(err)

	var name, value string
	//update env
	//1. mmcreco=1
	name = "mmcreco"
	value = "1"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//2. mmcpart=2
	name = "mmcpart"
	value = "2"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//3. snappy_boot
	name = "snappy_boot"
	value = "if test \"${snap_mode}\" = \"try\"; then setenv snap_mode \"trying\"; saveenv; if test \"${snap_try_core}\" != \"\"; then setenv snap_core \"${snap_try_core}\"; fi; if test \"${snap_try_kernel}\" != \"\"; then setenv snap_kernel \"${snap_try_kernel}\"; fi; elif test \"${snap_mode}\" = \"trying\"; then setenv snap_mode \"\"; saveenv; elif test \"${snap_mode}\" = \"recovery\"; then setenv loadinitrd \"load mmc ${mmcdev}:${mmcreco} ${initrd_addr} ${initrd_file}; setenv initrd_size ${filesize}\"; setenv loadkernel \"load mmc ${mmcdev}:${mmcreco} ${loadaddr} ${kernel_file}\"; setenv factory_recovery \"run loadfiles; setenv mmcroot \"/dev/disk/by-label/writable ${snappy_cmdline} snap_core=${snap_core} snap_kernel=${snap_kernel} recoverytype=factory_restore\"; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} 0x02000000\"; echo \"RECOVERY\"; run factory_recovery; fi; run loadfiles; setenv mmcroot \"/dev/disk/by-label/writable ${snappy_cmdline} snap_core=${snap_core} snap_kernel=${snap_kernel}\"; run mmcargs; bootz ${loadaddr} ${initrd_addr}:${initrd_size} 0x02000000"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//4. loadbootenv (load uboot.env from system-boot, because snapd always update uboot.env in system-boot while os/kernel snap updated)
	name = "loadbootenv"
	value = "load ${devtype} ${devnum}:${mmcpart} ${loadaddr} ${bootenv}"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)

	//5. bootenv (for system-boot/uboot.env)
	name = "bootenv"
	value = "uboot.env"
	env.Set(name, value)
	err = env.Save()
	rplib.Checkerr(err)
}

func usbhid() {
	log.Println("modprobe hid-generic and usbhid for usb keyboard")
	rplib.Shellexec("modprobe", "usbhid")
	rplib.Shellexec("modprobe", "hid-generic")
}

var configs rplib.ConfigRecovery

func main() {
	const GRUB_EDITENV = "grub-editenv"
	const LOG_PATH = "/writable/system-data/var/log/recovery/log.txt"
	const ASSERTION_FOLDER = "/writable/recovery"
	const ASSERTION_BACKUP_FOLDER = "/tmp/assert_backup"

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if "" == version {
		version = Version
	}

	commitstampInt64, _ := strconv.ParseInt(commitstamp, 10, 64)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	flag.Parse()
	log.Println("flag: ", flag.Args())

	if len(flag.Args()) != 2 {
		log.Fatal(fmt.Sprintf("Need two arguments. [RECOVERY_TYPE] and [RECOVERY_LABEL]. Current arguments: %v", flag.Args()))
	}

	usbhid()

	// Load config.yaml
	err := configs.Load("/recovery/config.yaml")
	rplib.Checkerr(err)
	log.Println(configs)

	// TODO: use enum to represent RECOVERY_TYPE
	var RECOVERY_TYPE, RECOVERY_LABEL = flag.Arg(0), flag.Arg(1)
	log.Println("RECOVERY_TYPE: ", RECOVERY_TYPE)
	log.Println("RECOVERY_LABEL: ", RECOVERY_LABEL)

	// find device name
	recovery_part := rplib.Findfs(fmt.Sprintf("LABEL=%s", RECOVERY_LABEL))
	log.Println("recovery_part: ", recovery_part)
	device, err := rplib.FindDevice(recovery_part)
	rplib.Checkerr(err)
	log.Println("device: ", device)

	// TODO: verify the image

	// If this is user triggered factory restore (first time is in factory and should happen automatically), ask user for confirm.
	if rplib.FACTORY_RESTORE == RECOVERY_TYPE {
		ioutil.WriteFile("/proc/sys/kernel/printk", []byte("0 0 0 0"), 0644)
		// io.WriteString(stdin, "Factory Restore will delete all user data, are you sure? [y/N] ")

		fmt.Println("Factory Restore will delete all user data, are you sure? [y/N] ")
		var response string
		fmt.Scanf("%s\n", &response)
		ioutil.WriteFile("/proc/sys/kernel/printk", []byte("4 4 1 7"), 0644)

		log.Println("response:", response)
		if "y" != response && "Y" != response {
			os.Exit(1)
		}
	}

	switch RECOVERY_TYPE {
	case rplib.FACTORY_RESTORE:
		// back up serial assertion
		writable_part := rplib.Findfs("LABEL=writable")
		err = os.MkdirAll("/tmp/writable/", 0755)
		rplib.Checkerr(err)
		err = syscall.Mount(writable_part, "/tmp/writable/", "ext4", 0, "")
		rplib.Checkerr(err)
		// back up assertion if ever signed
		if _, err := os.Stat(filepath.Join("/tmp/", ASSERTION_FOLDER)); err == nil {
			rplib.Shellexec("cp", "-ar", filepath.Join("/tmp/", ASSERTION_FOLDER), ASSERTION_BACKUP_FOLDER)
		}
		syscall.Unmount("/tmp/writable", 0)
	}

	if configs.Configs.PartitionType == "gpt" {
		log.Println("[recover the backup GPT entry at end of the disk.]")
		rplib.Shellexec("sgdisk", device, "--randomize-guids", "--move-second-header")
		log.Println("[recreate gpt partition table.]")
	}

	recovery_nr, recovery_end := getRecoveryPartition(device, RECOVERY_LABEL)

	// rebuild the partitions
	log.Println("[rebuild the partitions]")
	recreateRecoveryPartition(device, RECOVERY_LABEL, recovery_nr, recovery_end)

	// stream log to stdout and writable partition
	writable_part := rplib.Findfs("LABEL=writable")
	err = os.MkdirAll("/tmp/writable/", 0755)
	rplib.Checkerr(err)
	err = syscall.Mount(writable_part, "/tmp/writable/", "ext4", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount("/tmp/writable", 0)
	rootdir := "/tmp/writable/system-data/"

	logfile := filepath.Join("/tmp/", LOG_PATH)
	err = os.MkdirAll(path.Dir(logfile), 0755)
	rplib.Checkerr(err)
	log_writable, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0600)
	rplib.Checkerr(err)
	f := io.MultiWriter(log_writable, os.Stdout)
	log.SetOutput(f)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// FIXME, if grub need to support

	log.Println("[Add snaps for oem]")
	os.MkdirAll(filepath.Join(rootdir, "/var/lib/oem/"), 0755)
	rplib.Shellexec("cp", "-a", "/recovery/factory/snaps", filepath.Join(rootdir, "/var/lib/oem/"))
	rplib.Shellexec("cp", "-a", "/recovery/factory/snaps-devmode", filepath.Join(rootdir, "/var/lib/oem/"))

	// add firstboot service
	const MULTI_USER_TARGET_WANTS_FOLDER = "/etc/systemd/system/multi-user.target.wants/"
	log.Println("[Add FIRSTBOOT service]")
	rplib.Shellexec("/recovery/bin/rsync", "-a", "--exclude='.gitkeep'", filepath.Join("/recovery/factory", RECOVERY_TYPE)+"/", rootdir+"/")
	rplib.Shellexec("ln", "-s", "/lib/systemd/system/devmode-firstboot.service", filepath.Join(rootdir, MULTI_USER_TARGET_WANTS_FOLDER, "devmode-firstboot.service"))
	ioutil.WriteFile(filepath.Join(rootdir, "/var/lib/devmode-firstboot/conf.sh"), []byte(fmt.Sprintf("RECOVERYFSLABEL=\"%s\"\nRECOVERY_TYPE=\"%s\"\n", RECOVERY_LABEL, RECOVERY_TYPE)), 0644)

	switch RECOVERY_TYPE {
	case rplib.FACTORY_INSTALL:
		log.Println("[EXECUTE FACTORY INSTALL]")

		log.Println("[set next recoverytype to factory_restore]")
		rplib.Shellexec("mount", "-o", "rw,remount", "/recovery_partition")

		log.Println("[Start serial vault]")
		interface_list := strings.Split(rplib.Shellcmdoutput("ip -o link show | awk -F': ' '{print $2}'"), "\n")
		log.Println("interface_list:", interface_list)

		var net = 0
		for ; net < len(interface_list); net++ {
			if strings.Contains(interface_list[net], "eth") == true || strings.Contains(interface_list[net], "enx") == true {
				break
			}
		}
		if net == len(interface_list) {
			panic(fmt.Sprintf("Need one ethernet interface to connect to identity-vault. Current network interface: %v", interface_list))
		}
		eth := interface_list[net] // select nethernet interface.
		rplib.Shellexec("ip", "link", "set", "dev", eth, "up")
		rplib.Shellexec("dhclient", "-1", eth)

		vaultServerIP := rplib.Shellcmdoutput("ip route | awk '/default/ { print $3 }'") // assume identity-vault is hosted on the gateway
		log.Println("vaultServerIP:", vaultServerIP)

		// TODO: read assertion information from gadget snap
		if !configs.Recovery.SignSerial {
			log.Println("Will not sign serial")
			break
		}
		// TODO: Start signing serial
		log.Println("Start signing serial")
	case rplib.FACTORY_RESTORE:
		log.Println("[User restores the system]")
		// restore assertion if ever signed
		if _, err := os.Stat(ASSERTION_BACKUP_FOLDER); err == nil {
			log.Println("Restore gpg key and serial")
			rplib.Shellexec("cp", "-ar", ASSERTION_BACKUP_FOLDER, filepath.Join("/tmp/", ASSERTION_FOLDER))
		}
	}

	// update uboot env
	system_boot_part := rplib.Findfs("LABEL=system-boot")
	log.Println("system_boot_part:", system_boot_part)
	err = os.MkdirAll("/tmp/system-boot", 0755)
	rplib.Checkerr(err)
	err = syscall.Mount(system_boot_part, "/tmp/system-boot", "vfat", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount("/tmp/system-boot", 0)

	log.Println("Update uboot env(ESP/system-boot)")
	//fsck needs ignore error code
	cmd := exec.Command("fsck", "-y", fmt.Sprintf("/dev/disk/by-label/%s", RECOVERY_LABEL))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
	rplib.Shellexec("mount", "-o", "remount,rw", fmt.Sprintf("/dev/disk/by-label/%s", RECOVERY_LABEL), "/recovery_partition")
	hack_uboot_env("/recovery_partition/uboot.env")
	rplib.Shellexec("mount", "-o", "remount,ro", fmt.Sprintf("/dev/disk/by-label/%s", RECOVERY_LABEL), "/recovery_partition")
	hack_uboot_env("/tmp/system-boot/uboot.env")

	//release dhclient
	rplib.Shellexec("dhclient", "-x")
	rplib.Sync()
}
