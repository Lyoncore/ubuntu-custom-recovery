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
if [ ! -d $ROOTFSMNT/etc/cloud/cloud.cfg.d/ ]; then
    mkdir -p $ROOTFSMNT/etc/cloud/cloud.cfg.d
fi

if [ -d \$RECOVERYMNT/system-data/etc/cloud/cloud.cfg.d ]; then
    cp \$RECOVERYMNT/system-data/etc/cloud/cloud.cfg.d/* \$ROOTFSMNT/etc/cloud/cloud.cfg.d/
fi

# move ubuntu and factory restore boot entries the last two
PATH=\$PATH:\$RECOVERYMNT/recovery/bin
IN=\$(LD_LIBRARY_PATH=\$RECOVERYMNT/recovery/lib efibootmgr | grep BootOrder | cut -d ':' -f 2 | tr -d '[:space:]')
OLDIFS=\$IFS
IFS=","

# remove the unsed bootentries for maas image
ENTRY=\$(LD_LIBRARY_PATH=\$RECOVERYMNT/recovery/lib efibootmgr | grep factory_restore | cut -d "*" -f 1)
if [ ! -z "\$ENTRY" ]; then
    FACTORY_RESTORE_BOOT_ENTRY=\${ENTRY#Boot}
fi
ENTRY=\$(LD_LIBRARY_PATH=\$RECOVERYMNT/recovery/lib efibootmgr | grep -i ubuntu | cut -d "*" -f 1)
if [ ! -z "\$ENTRY" ]; then
    UBUNTU_BOOT_ENTRY=\${ENTRY#Boot}
fi

for x in \$IN
do
        if [ "\$x" != "\$FACTORY_RESTORE_BOOT_ENTRY" ] && [ "\$x" != "\$UBUNTU_BOOT_ENTRY" ]; then
            if [ "\$new_boot" = "" ]; then
                new_boot=\$x
            else
                new_boot="\$new_boot,\$x"
            fi
        fi
done

if [ "\$UBUNTU_BOOT_ENTRY" != "" ]; then
    new_boot="\$new_boot,\$UBUNTU_BOOT_ENTRY"
fi
if [ \$FACTORY_RESTORE_BOOT_ENTRY != "" ]; then
    new_boot="\$new_boot,\$FACTORY_RESTORE_BOOT_ENTRY"
fi

IFS=\$OLDIFS

LD_LIBRARY_PATH=\$RECOVERYMNT/recovery/lib efibootmgr -o \$new_boot

# workaround: not using networkmanager for cloud-init, or it will not get IP before cloud-init-local.service
sed -i "s/renderer: NetworkManager/renderer: networkd/g" \$ROOTFSMNT/etc/netplan/00-default-nm-renderer.yaml
# set networkmanager back after cloud-init complete
cat > \$ROOTFSMNT/etc/cloud/cloud.cfg.d/90-workaound-back-to-nm.cfg << EOF
runcmd:
 - [ sed, -i, "s/renderer: networkd/renderer: NetworkManager/g", /etc/netplan/00-default-nm-renderer.yaml ]
 - [ netplan, generate ]
 - [ systemctl, restart, snap.network-manager.networkmanager.service ]
 - [ netplan, apply ]
EOF
chmod a+x \$ROOTFSMNT/etc/cloud/cloud.cfg.d/90-workaound-back-to-nm.cfg

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

# let PXE keeps in the first boot entries
IN=\$(chroot \$ROOTFSMNT /bin/bash -c "efibootmgr | grep BootOrder | cut -d ':' -f 2 | tr -d '[:space:]'")
OLDIFS=\$IFS
IFS=","
LAST_PXE_BOOT=6

for x in \$IN
do
        count=\$((count+1))
        if [ \$count -eq 1 ]; then
                moved_boot=\$x
        elif [ \$count -eq 2 ]; then
                moved_boot="\$moved_boot,\$x"
        elif [ \$count -eq \$LAST_PXE_BOOT ]; then
                new_boot="\$new_boot,\$x,\$moved_boot"
        else
                if [ ! -z "\$new_boot" ]; then
                        new_boot="\$new_boot,\$x"
                else
                        new_boot="\$x"
                fi
        fi
        #echo "\$count: \$x"
done
IFS=\$OLDIFS

chroot \$ROOTFSMNT efibootmgr -o \$new_boot

# workaround: not using networkmanager for cloud-init, or it will not get IP before cloud-init-local.service
sed -i "s/renderer: NetworkManager/renderer: networkd/g" \$ROOTFSMNT/etc/netplan/01-network-manager-all.yaml
# set networkmanager back after cloud-init complete
cat > \$ROOTFSMNT/etc/cloud/cloud.cfg.d/90-workaound-back-to-nm.cfg << EOF
runcmd:
 - [ sed, -i, "s/renderer: networkd/renderer: NetworkManager/g", /etc/netplan/01-network-manager-all.yaml ]
 - [ netplan, generate ]
 - [ systemctl, restart, network-manager.service ]
 - [ netplan, apply ]
EOF
chmod a+x \$ROOTFSMNT/etc/cloud/cloud.cfg.d/90-workaound-back-to-nm.cfg

umount \$ROOTFSMNT/sys
umount \$ROOTFSMNT/proc
umount \$ROOTFSMNT/dev
umount $ROOTFSMNT/run

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
