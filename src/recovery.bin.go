package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/snapcore/snapd/asserts"
)

import rplib "github.com/Lyoncore/ubuntu-recovery-rplib"

var version string
var commit string
var commitstamp string
var build_date string

func getRecoveryPartition(device string, RECOVERY_LABEL string) (recovery_nr, recovery_end int) {
	var err error
	const OLD_PARTITION = "/tmp/old-partition.txt"
	recovery_nr = -1

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
		begin := strings.TrimRight(fields[1], "B")
		end, err := strconv.Atoi(strings.TrimRight(fields[2], "B"))
		rplib.Checkerr(err)
		size := strings.TrimRight(fields[3], "B")
		fstype := fields[4]
		label := fields[5]
		log.Println("nr: ", nr)
		log.Println("begin: ", begin)
		log.Println("end: ", end)
		log.Println("size: ", size)
		log.Println("fstype: ", fstype)
		log.Println("label: ", label)

		if RECOVERY_LABEL == label {
			recovery_nr = nr
			recovery_end = end
			log.Println("recovery_nr:", recovery_nr)
			log.Println("recovery_end:", recovery_end)
			continue
		}

		if -1 != recovery_nr {
			// delete all the partitions after recovery partition
			rplib.Shellexec("parted", "-ms", device, "rm", fmt.Sprintf("%v", nr))
		}
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

func recreateRecoveryPartition(device string, RECOVERY_LABEL string, recovery_nr int, recovery_end int) (normal_boot_nr int) {
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
		// TODO: allocate partition according to gadget.PartitionLayout
		// create the new partition
		if "writable" == partition {
			end_size = "-1M"
			fstype = "ext4"
		} else if "system-boot" == partition {
			size := 64 * 1024 * 1024 // 64 MB in Bytes
			end_size = strconv.Itoa(last_end+size) + "B"
			fstype = "fat32"
		}
		log.Println("end_size:", end_size)

		rplib.Shellexec("parted", "-a", "optimal", "-ms", device, "--", "mkpart", "primary", fstype, fmt.Sprintf("%vB", last_end+1), end_size, "name", fmt.Sprintf("%v", nr), partition)
		_, new_end := rplib.GetPartitionBeginEnd(device, nr)

		if "system-boot" == partition {
			normal_boot_nr = nr
			log.Println("normal_boot_nr:", normal_boot_nr)
		}

		rplib.Shellexec("udevadm", "settle")
		rplib.Shellexec("parted", "-ms", device, "unit", "B", "print") // debug

		block := fmt.Sprintf("%s%d", device, nr)
		log.Println("block:", block)
		switch partition {
		case "writable":
			rplib.Shellexec("mkfs.ext4", "-F", "-L", "writable", block)
			os.MkdirAll("/tmp/writable/", 0644)
			err := syscall.Mount(block, "/tmp/writable", "ext4", 0, "")
			rplib.Checkerr(err)
			defer syscall.Unmount("/tmp/writable", 0)
			rplib.Shellexec("tar", "--xattrs", "-xJvpf", "/recovery/factory/writable.tar.xz", "-C", "/tmp/writable/")
		case "system-boot":
			rplib.Shellexec("mkfs.vfat", "-F", "32", "-n", "system-boot", block)
			os.MkdirAll("/tmp/system-boot/", 0644)
			err := syscall.Mount(block, "/tmp/system-boot", "vfat", 0, "")
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

func hack_grub_cfg(grub_cfg string, recovery_type_cfg string, recovery_type_label string, recovery_part_label string) {
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

func usbhid() {
	log.Println("modprobe hid-generic and usbhid for usb keyboard")
	rplib.Shellexec("modprobe", "usbhid")
	rplib.Shellexec("modprobe", "hid-generic")
}

func main() {
	const LOG_PATH = "/writable/system-data/var/log/recovery/log.txt"
	const ASSERTION_FOLDER = "/writable/recovery"
	const ASSERTION_BACKUP_FOLDER = "/tmp/assert_backup"

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	commitstampInt64, _ := strconv.ParseInt(commitstamp, 10, 64)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	flag.Parse()
	log.Println("flag: ", flag.Args())

	if len(flag.Args()) != 2 {
		log.Fatal(fmt.Sprintf("Need two arguments. [RECOVERY_TYPE] and [RECOVERY_LABEL]. Current arguments: %v", flag.Args()))
	}

	usbhid()

	// TODO: use enum to represent RECOVERY_TYPE
	var RECOVERY_TYPE, RECOVERY_LABEL = flag.Arg(0), flag.Arg(1)
	log.Println("RECOVERY_TYPE: ", RECOVERY_TYPE)
	log.Println("RECOVERY_LABEL: ", RECOVERY_LABEL)

	// find device name
	recovery_part := rplib.Findfs(fmt.Sprintf("LABEL=%s", RECOVERY_LABEL))
	log.Println("recovery_part: ", recovery_part)
	syspath := path.Dir(rplib.Realpath(fmt.Sprintf("/sys/class/block/%s", path.Base(recovery_part))))
	log.Println("syspath: ", syspath)

	dat, err := ioutil.ReadFile(fmt.Sprintf("%s/dev", syspath))
	rplib.Checkerr(err)
	dat_str := strings.TrimSpace(string(dat))
	log.Println("dat_str: ", dat_str)
	device := rplib.Realpath(fmt.Sprintf("/dev/block/%s", dat_str))
	log.Println("device: ", device)

	// TODO: verify the image

	// If this is user triggered factory restore (first time is in factory and should happen automatically), ask user for confirm.
	if RECOVERY_TYPE == "restore" {
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
	case "restore":
		// back up serial assertion
		writable_part := rplib.Findfs("LABEL=writable")
		os.MkdirAll("/tmp/writable/", 0644)
		err = syscall.Mount(writable_part, "/tmp/writable/", "ext4", 0, "")
		rplib.Checkerr(err)
		rplib.Shellexec("cp", "-ar", "/tmp/"+ASSERTION_FOLDER, ASSERTION_BACKUP_FOLDER)
		syscall.Unmount("/tmp/writable", 0)
	}
	log.Println("[recover the backup GPT entry at end of the disk.]")
	rplib.Shellexec("sgdisk", device, "--randomize-guids", "--move-second-header")

	log.Println("[recreate gpt partition table.]")

	recovery_nr, recovery_end := getRecoveryPartition(device, RECOVERY_LABEL)

	// rebuild the partitions
	log.Println("[rebuild the partitions]")
	normal_boot_nr := recreateRecoveryPartition(device, RECOVERY_LABEL, recovery_nr, recovery_end)

	// stream log to stdout and writable partition
	writable_part := rplib.Findfs("LABEL=writable")
	os.MkdirAll("/tmp/writable/", 0644)
	err = syscall.Mount(writable_part, "/tmp/writable/", "ext4", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount("/tmp/writable", 0)

	logfile := "/tmp/" + LOG_PATH
	err = os.MkdirAll(path.Dir(logfile), 0644)
	rplib.Checkerr(err)
	log_writable, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0600)
	rplib.Checkerr(err)
	f := io.MultiWriter(log_writable, os.Stdout)
	log.SetOutput(f)
	log.Printf("Version: %v, Commit: %v, Build date: %v\n", version, commit, time.Unix(commitstampInt64, 0).UTC())

	// add grub entry
	system_boot_part := rplib.Findfs("LABEL=system-boot")
	log.Println("system_boot_part:", system_boot_part)
	os.MkdirAll("/tmp/system-boot", 0644)
	err = syscall.Mount(system_boot_part, "/tmp/system-boot", "vfat", 0, "")
	rplib.Checkerr(err)
	defer syscall.Unmount("/tmp/system-boot", 0)

	log.Println("add grub entry")
	hack_grub_cfg("/tmp/system-boot/EFI/ubuntu/grub/grub.cfg", "factory_restore", "Factory Restore", RECOVERY_LABEL)
	rplib.Shellexec("mount", "-o", "ro,remount", "/tmp/system-boot")

	// remove past uefi entry
	log.Println("[remove past uefi entry]")
	const EFIBOOTMGR = "efibootmgr"
	entries := rplib.GetBootEntries("factory_restore")
	for _, entry := range entries {
		rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
	}
	entries = rplib.GetBootEntries("snappy_ubuntu_core")
	for _, entry := range entries {
		rplib.Shellexec(EFIBOOTMGR, "-b", entry, "-B")
	}

	// add new uefi entry
	log.Println("[add new uefi entry]")
	const LOADER = "\\EFI\\BOOT\\BOOTX64.EFI"
	rplib.CreateBootEntry(device, recovery_nr, LOADER, "factory_restore")
	rplib.CreateBootEntry(device, normal_boot_nr, LOADER, "snappy_ubuntu_core")

	const GRUB_EDITENV = "grub-editenv"
	switch RECOVERY_TYPE {
	case "install":
		log.Println("[EXECUTE FACTORY INSTALL]")

		log.Println("[set next recoverytype to factory_restore]")
		rplib.Shellexec("mount", "-o", "rw,remount", "/recovery_partition")
		log.Println("set recoverytype")
		rplib.Shellexec(GRUB_EDITENV, "/recovery_partition/efi/ubuntu/grub/grubenv", "set", "recoverytype=factory_restore")

		log.Println("[Start serial vault]")

		// modprobe ethernet driver
		rplib.Shellexec("modprobe", "r8169")
		rplib.Shellexec("modprobe", "e1000")
		interface_list := strings.Split(rplib.Shellcmdoutput("ip -o link show | awk -F': ' '{print $2}'"), "\n")
		log.Println("interface_list:", interface_list)
		if len(interface_list) < 2 {
			log.Fatal(fmt.Sprintf("Need one network interface to connect to identity-vault. Current network interface: %v", interface_list))
		}
		eth := interface_list[1] // select first non "lo" network interface.
		rplib.Shellexec("ip", "link", "set", "dev", eth, "up")
		rplib.Shellexec("dhclient", eth)

		vaultServerIP := rplib.Shellcmdoutput("ip route | awk '/default/ { print $3 }'") // assume identity-vault is hosted on the gateway
		log.Println("vaultServerIP:", vaultServerIP)

		rplib.Shellexec("/recovery/bin/rngd", "-r", "/dev/urandom")

		// TODO: read assertion information from gadget snap
		fileContent, err := ioutil.ReadFile("/recovery/assertions/model.txt")
		rplib.Checkerr(err)
		modelAssertion, err := asserts.Decode(fileContent)
		rplib.Checkerr(err)

		rplib.SignSerial(modelAssertion, "/tmp/"+ASSERTION_FOLDER, fmt.Sprintf("http://%s:8080/1.0/sign", vaultServerIP))
	case "restore":
		log.Println("[Use restores the system]")
		log.Println("Restore gpg key and serial")
		rplib.Shellexec("cp", "-ar", ASSERTION_BACKUP_FOLDER, "/tmp/"+ASSERTION_FOLDER)
	}

	rplib.Sync()
}
