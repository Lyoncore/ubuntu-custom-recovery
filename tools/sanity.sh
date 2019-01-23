#!/bin/bash
#
# Copyright (C) 2018 Canonical Ltd
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3 as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )

set -ex
env

export PATH=$PATH:/sbin

OUTDIR=$DIR/outdir

print_help()
{
    echo "usage: ./sanity.sh --source-image=<path to image>/<image name> [--output-dir=<folder>]"
}

# Handle commandline parameters
while [ -n "$1" ]; do
    case "$1" in
        --output-dir=*)
            OUTDIR=${1#*=}
            ;;
        --source-image=*)
            SRC_IMG=${1#*=}
            ;;
        -h | --help )
            print_help
            exit 1
            ;;
        * )
            echo "ERROR: unknown option $1"
            print_help
            exit 1
            ;;
    esac
    shift
done

if [ ! -f $SRC_IMG ]; then
     echo "source image not found($SRC_IMG)"
     exit 1
fi

echo "The source image:$SRC_IMG"
SRC_IMG_NAME=$(basename $SRC_IMG)
MAAS_IMG_XZ="maas-$SRC_IMG_NAME"
MAAS_IMG=${MAAS_IMG_XZ::-3}

cp $SRC_IMG $MAAS_IMG_XZ
pxz -d $MAAS_IMG_XZ

# update the partition label for maas image
LOOP_IMG=$(sudo losetup --find --show $MAAS_IMG | xargs basename)
sudo kpartx -avs /dev/$LOOP_IMG
sudo fatlabel /dev/mapper/${LOOP_IMG}p1 ESP

mkdir img_mnt || true
sudo mount /dev/mapper/${LOOP_IMG}p1 img_mnt

# detect image is Ubuntu Core or Ubuntu Server
# the Ubuntu Core includes *.snap in recovery partition
UC=$(ls img_mnt/*.snap 2>/dev/null) || true

if [ "$UC" != "" ]; then
    #the hook script for Ubuntu Core
    sudo bash -c 'cat > img_mnt/recovery/factory/OEM_post_install_hook/60-maas-hook.sh << EEF
#!/bin/bash

set -x
RECOVERYMNT="/run/recovery"
ROOTFSMNT="/writable/system-data"

# copy maas cloud-init config files
if [ -d \$RECOVERYMNT/system-data/etc/cloud/cloud.cfg.d ]; then
    if [ ! -d \$ROOTFSMNT/etc/cloud/cloud.cfg.d/ ]; then
        mkdir -p \$ROOTFSMNT/etc/cloud/cloud.cfg.d/
    fi
    cp \$RECOVERYMNT/system-data/etc/cloud/cloud.cfg.d/* \$ROOTFSMNT/etc/cloud/cloud.cfg.d/
fi

# disable cloud-init network setting
echo "network: {config: disabled}" > \$ROOTFSMNT/etc/cloud/cloud.cfg.d/99-disable-network-config.cfg
if [ -f \$ROOTFSMNT/etc/netplan/50-cloud-init.yaml ]; then
    rm \$ROOTFSMNT/etc/netplan/50-cloud-init.yaml
fi

# move ubuntu and factory restore boot entries the last two
PATH=\$PATH:\$RECOVERYMNT/recovery/bin
IN=\$(LD_LIBRARY_PATH=\$RECOVERYMNT/recovery/lib efibootmgr | grep BootOrder | cut -d ':' -f 2 | tr -d '[:space:]')
OLDIFS=\$IFS
IFS=","

# here to clean the UsbInvocation log, for PXE boot
if [ ! -d \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/ ]; then
    mkdir -p \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/
fi

cat > \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/52-workarond-reporting-maas.sh << EOF
#!/bin/sh
set -x

cd /var/lib/cloud
rm -rf data handlers instance instances seed sem
systemctl restart cloud-init

echo > /tmp/52-workarond-reporting-maas-done
EOF
chmod +x \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/52-workarond-reporting-maas.sh

cat > \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/50-clean-usbinvocation-log.sh << EOF
#!/bin/sh

if [ -e /dev/sda1 ]; then
        mount /dev/sda1 /mnt
        if [ -f /mnt/UsbInvocationScript.txt ]; then
                find /mnt/ -maxdepth 1 "!" -name "UsbInvocationScript*"  -name "*.txt" -delete
        fi
        umount /mnt
fi
echo > /tmp/clean-usbinvocation-log-done
EOF
chmod +x \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/50-clean-usbinvocation-log.sh

cat > \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/51-remove-ufi-bootentry-for-maas.sh << EOF
#!/bin/sh
set -ex
BOOT=\\\$(efibootmgr  | grep -i ubuntu | cut -d " " -f 1 | tr -d "Boot *")
while [ ! -z "\\\$BOOT" ]; do
    efibootmgr -B -b \\\$BOOT
    BOOT=\\\$(efibootmgr  | grep -i ubuntu | cut -d " " -f 1 | tr -d "Boot *")
done

BOOT=\\\$(efibootmgr  | grep factory_restore | cut -d " " -f 1 | tr -d "Boot *")
if [ ! -z "\\\$BOOT" ]; then
    efibootmgr -B -b \\\$BOOT
fi

echo > /tmp/51-remove-ufi-bootentry-for-maas-done
EOF
chmod +x \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/51-remove-ufi-bootentry-for-maas.sh

# the boot partition needs modification for maas bootup
mkdir /tmp/boot
mount /dev/disk/by-label/system-boot /tmp/boot
cp /tmp/boot/EFI/BOOT/* /tmp/boot/EFI/UBUNTU/
umount /tmp/boot
EEF'
else
    # Adding a post install hook for maas
    sudo bash -c 'cat > img_mnt/recovery/factory/OEM_post_install_hook/60-maas-hook.sh << EEF
#!/bin/sh

set -x
RECOVERYMNT="/run/recovery"
ROOTFSMNT="/target"

mount --bind /sys \$ROOTFSMNT/sys
mount --bind /proc \$ROOTFSMNT/proc
mount --bind /dev \$ROOTFSMNT/dev
mount --bind /run \$ROOTFSMNT/run

chroot \$ROOTFSMNT adduser --quiet --disabled-password --shell /bin/bash --home /home/test --gecos "test" test
chroot \$ROOTFSMNT sudo usermod -aG sudo test
chroot \$ROOTFSMNT sh -c "echo test:test | chpasswd"

#USER_ID=\$(chroot \$ROOTFSMNT id -u test)
#GROUP_ID=\$(chroot \$ROOTFSMNT id -g test)

#if [ ! -d \$ROOTFSMNT/home/test/.ssh ];then
#    mkdir \$ROOTFSMNT/home/test/.ssh
#    chown \$USER_ID:\$GROUP_ID \$ROOTFSMNT/home/test/.ssh
#    chmod 775 \$ROOTFSMNT/home/test/.ssh
#fi
#cp \$RECOVERYMNT/curtin/authorized_keys \$ROOTFSMNT/home/test/.ssh/
#chown \$USER_ID:\$GROUP_ID \$ROOTFSMNT/home/test/.ssh/authorized_keys
#chmod 600 \$ROOTFSMNT/home/test/.ssh/authorized_keys

# copy maas cloud-init config files
if [ -d \$RECOVERYMNT/system-data/etc/cloud/cloud.cfg.d ]; then
    if [ ! -d \$ROOTFSMNT/etc/cloud/cloud.cfg.d/ ]; then
        mkdir -p \$ROOTFSMNT/etc/cloud/cloud.cfg.d/
    fi
    cp \$RECOVERYMNT/system-data/etc/cloud/cloud.cfg.d/* \$ROOTFSMNT/etc/cloud/cloud.cfg.d/
fi

if [ ! -d \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/ ]; then
    mkdir -p \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/
fi

cat > \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/52-workarond-reporting-maas.sh << EOF
#!/bin/sh
set -x

cd /var/lib/cloud
rm -rf data handlers instance instances seed sem
systemctl restart cloud-init

echo > /tmp/52-workarond-reporting-maas-done
EOF
chmod +x \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/52-workarond-reporting-maas.sh

cat > \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/51-remove-ufi-bootentry-for-maas.sh << EOF
#!/bin/sh
set -ex
BOOT=\\\$(efibootmgr  | grep -i ubuntu | cut -d " " -f 1 | tr -d "Boot *")
while [ ! -z "\\\$BOOT" ]; do
    efibootmgr -B -b \\\$BOOT
    BOOT=\\\$(efibootmgr  | grep -i ubuntu | cut -d " " -f 1 | tr -d "Boot *")
done

BOOT=\\\$(efibootmgr  | grep factory_restore | cut -d " " -f 1 | tr -d "Boot *")
if [ ! -z "\\\$BOOT" ]; then
    efibootmgr -B -b \\\$BOOT
fi

echo > /tmp/51-remove-ufi-bootentry-for-maas-done
EOF
chmod +x \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/51-remove-ufi-bootentry-for-maas.sh

cat > \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/50-clean-usbinvocation-log.sh << EOF
#!/bin/sh

if [ -e /dev/sda1 ]; then
        mount /dev/sda1 /mnt
        if [ -f /mnt/UsbInvocationScript.txt ]; then
                find /mnt/ -maxdepth 1 "!" -name "UsbInvocationScript*"  -name "*.txt" -delete
        fi
        umount /mnt
fi
echo > /tmp/clean-usbinvocation-log-done
EOF
chmod +x \$ROOTFSMNT/var/lib/cloud/scripts/per-boot/50-clean-usbinvocation-log.sh

umount \$ROOTFSMNT/sys
umount \$ROOTFSMNT/proc
umount \$ROOTFSMNT/dev
umount \$ROOTFSMNT/run

EEF'
fi

# copy the maas assets
if [ -d maas-assets/ ]; then
    sudo cp -r maas-assets/* img_mnt/
fi

# The maas needs boot from EFI/UBUNTU/shimx64.efi, but recovery doesn't
# Here to create an EFI/UBUNTU/ boot assets
if [ ! -d img_mnt/EFI/UBUNTU/ ]; then
    sudo mkdir img_mnt/EFI/UBUNTU/
fi
sudo cp img_mnt/EFI/BOOT/* img_mnt/EFI/UBUNTU/
if [ ! -f img_mnt/EFI/UBUNTU/shimx64.efi ]; then
    if [ ! -f /usr/lib/shim/shimx64.efi.signed ];then
        sudo apt update
        sudo apt install -y shim-signed
    fi
    sudo cp /usr/lib/shim/shimx64.efi.signed img_mnt/EFI/UBUNTU/shimx64.efi
fi

# Cheat curtin in maas that is a ubuntu core image
# Create a /system-data/var/lib/snapd/ dir here
sudo mkdir -p img_mnt/system-data/var/lib/snapd/

sudo umount img_mnt

# If this is an ubuntu core image. that might be an writable partition exist in image.
# We need to clean /var/lib/snapd that to prevent maas deploying meet missing config files issue.
if [ -e /dev/mapper/${LOOP_IMG}p3 ]; then
    sudo mount /dev/mapper/${LOOP_IMG}p3 img_mnt
    sudo rm -rf img_mnt/system-data/var/lib/snapd/
    sudo umount img_mnt
fi
rmdir img_mnt

sudo kpartx -ds /dev/$LOOP_IMG
sudo losetup -d /dev/$LOOP_IMG

# compress the image
if [ ! -d $OUTDIR ]; then
    mkdir $OUTDIR
fi
mv $MAAS_IMG $OUTDIR/$MAAS_IMG
pxz $OUTDIR/$MAAS_IMG
sha256sum $OUTDIR/$MAAS_IMG_XZ > $OUTDIR/$MAAS_IMG".xz.sha256sum"

# generate a maas.env for jenkins
cat > $OUTDIR/maas.env << EOF
FILE_TYPE=ddxz
IMG_NAME=$MAAS_IMG_XZ
EOF
