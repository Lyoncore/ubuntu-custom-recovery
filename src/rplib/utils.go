package rplib

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	WritableImage = "writable_resized.e2fs"
)

func DD(input string, output string, args ...string) {
	args = append([]string{fmt.Sprintf("if=%s", input), fmt.Sprintf("of=%s", output)}, args...)
	Shellexec("dd", args...)
}

func Sync() {
	Shellexec("sync")
}

func Reboot() {
	Shellexec("reboot")
}

func FindDevice(blockDevice string) (device string, err error) {
	syspath := path.Dir(Realpath(filepath.Join("/sys/class/block", path.Base(blockDevice))))

	dat, err := ioutil.ReadFile(fmt.Sprintf("%s/dev", syspath))
	if err != nil {
		return "", err
	}
	dat_str := strings.TrimSpace(string(dat))
	device = Realpath(fmt.Sprintf("/dev/block/%s", dat_str))
	return device, nil
}

func Findfs(arg string) string {
	return Shellexecoutput("findfs", arg)
}

func Realpath(path string) string {
	newPath, err := filepath.Abs(path)
	if err != nil {
		log.Panic(err)
	}

	newPath, err = filepath.EvalSymlinks(newPath)
	if err != nil {
		log.Panic(err)
	}
	return newPath
}

func SetPartitionFlag(device string, nr int, flag string) {
	Shellexec("parted", "-ms", device, "set", fmt.Sprintf("%v", nr), flag, "on")
}

func BlockSize(block string) (size int64) {
	// unit Byte
	sizeStr := Shellexecoutput("blockdev", "--getsize64", block)
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	Checkerr(err)
	return
}

func GetPartitionSize(device string, nr int) (size int64) {
	var err error
	line := Shellcmdoutput(fmt.Sprintf("parted -ms %s unit B print | grep \"^%d:\"", device, nr))
	log.Printf("line:", line)
	fields := strings.Split(line, ":")
	size, err = strconv.ParseInt(strings.TrimRight(fields[3], "B"), 10, 64)
	Checkerr(err)
	return
}

func GetPartitionBeginEnd(device string, nr int) (begin, end int) {
	var err error
	line := Shellcmdoutput(fmt.Sprintf("parted -ms %s unit B print | grep \"^%d:\"", device, nr))
	log.Printf("line:", line)
	fields := strings.Split(line, ":")
	begin, err = strconv.Atoi(strings.TrimRight(fields[1], "B"))
	Checkerr(err)
	end, err = strconv.Atoi(strings.TrimRight(fields[2], "B"))
	Checkerr(err)
	return
}

func GetPartitionBeginEnd64(device string, nr int) (begin, end int64) {
	var err error
	line := Shellcmdoutput(fmt.Sprintf("parted -ms %s unit B print | grep \"^%d:\"", device, nr))
	log.Printf("line:", line)
	fields := strings.Split(line, ":")
	begin, err = strconv.ParseInt(strings.TrimRight(fields[1], "B"), 10, 64)
	Checkerr(err)
	end, err = strconv.ParseInt(strings.TrimRight(fields[2], "B"), 10, 64)
	Checkerr(err)
	return
}

func GetBootEntries(keyword string) (entries []string) {
	entryStr := Shellcmdoutput(fmt.Sprintf("efibootmgr -v | grep \"%s\" | cut -f 1 | sed 's/[^0-9]*//g'", keyword))
	log.Printf("entryStr: [%s]\n", entryStr)
	if "" == entryStr {
		entries = []string{}
	} else {
		entries = strings.Split(entryStr, "\n")
	}
	log.Printf("entries:", entries)
	return
}

func CreateBootEntry(device string, partition int, loader string, label string) {
	Shellexec("efibootmgr", "-c", "-d", device, "-p", fmt.Sprintf("%v", partition), "-l", loader, "-L", label)
}

func ReadKernelCmdline() string {
	data, err := ioutil.ReadFile("/proc/cmdline")
	Checkerr(err)
	cmdline := string(data)
	return cmdline
}

func IsKernelCmdlineContains(substr string) bool {
	return strings.Contains(ReadKernelCmdline(), substr)
}

// SymlinkCopy() copies symlink to distination.
// If dst is a directory, it makes a copy to and keep same name.
// If dst is a new name, it makes a rename copy.
func SymlinkCopy(src, dst string) (err error) {
	var dst_name string
	//Check src exist and is a symlink
	srcStat, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcStat.Mode()&os.ModeSymlink != os.ModeSymlink {
		err = errors.New("Source is not symlink")
		return err
	}
	//Check dst not exist for copy
	dstStat, err := os.Stat(dst)
	if err == nil && dstStat.IsDir() == true {
		dst_name = fmt.Sprintf("%s/%s", dst, filepath.Base(src))
	} else {
		dst_name = dst //it's destination with file name
	}

	link_src, err := os.Readlink(src)
	if err != nil {
		return err
	}
	return os.Symlink(link_src, dst_name)
}

// FileCopy() copies source file to distination.
// If dst is a directory, it makes a copy to and keep same name.
// If dst is a new name, it makes a rename copy.
func FileCopy(src, dst string) (err error) {
	var dst_name string
	//Check src exist for copy, and cannot be a dir
	srcStat, err := os.Stat(src)
	if err != nil || srcStat.IsDir() == true {
		return err
	}

	//Check dst not exist for copy
	dstStat, err := os.Stat(dst)
	if err == nil && dstStat.IsDir() == true {
		dst_name = fmt.Sprintf("%s/%s", dst, filepath.Base(src))
	} else {
		dst_name = dst //it's destination with file name
	}

	srcf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcf.Close()
	dstf, err := os.Create(dst_name)
	if err != nil {
		return err
	}
	defer dstf.Close()

	_, err = io.Copy(dstf, srcf)
	if err == nil {
		err = os.Chmod(dst_name, srcStat.Mode())
	}
	return
}

type copyinfo struct {
	dst      string
	basepath string
}

//walkCopy() is for walk() in CopyTree()
func walkCopy(ci copyinfo) filepath.WalkFunc {
	return func(src string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		srcf, err := filepath.Rel(ci.basepath, src)
		dst := fmt.Sprintf("%s/%s", ci.dst, srcf)
		if info.IsDir() {
			if srcf == "." {
				return nil
			}
			err = os.MkdirAll(fmt.Sprintf("%s/%s", ci.dst, srcf), info.Mode())
			if err != nil {
				return nil
			}
		} else if info.Mode()&os.ModeSymlink == os.ModeSymlink { //It's symlink
			return SymlinkCopy(src, dst)
		} else { //It's file
			return FileCopy(src, dst)
		}

		return nil
	}
}

// CopyTree() copy the source directory recursivly to distination.
// The src and dst must be a path to directory.
// If dst directory not exist, it will create a new to copy to.
func CopyTree(src, dst string) (err error) {
	//Check src exist and must be a directory
	srcStat, err := os.Stat(src)
	if err != nil || srcStat.IsDir() == false {
		err = errors.New("Source must be a directory")
		return err
	}

	//Check dst exist must be a dir
	dstStat, err := os.Stat(dst)
	if err == nil && dstStat.IsDir() == false {
		err = errors.New("Distination must be a directory")
		return err
	} else { //make a new dir to copy
		err = os.MkdirAll(dst, srcStat.Mode())
		if err != nil {
			return err
		}
	}

	ci := copyinfo{dst, src}
	return filepath.Walk(src, walkCopy(ci))
}
