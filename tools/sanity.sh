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

#chroot \$ROOTFSMNT adduser --quiet --disabled-password --shell /bin/bash --home /home/test --gecos "test" test
#chroot \$ROOTFSMNT sudo usermod -aG sudo test
#chroot \$ROOTFSMNT sh -c "echo test:test | chpasswd"

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

# copy the maas assets
if [ -d maas-assets/ ]; then
    sudo cp -r maas-assets/* img_mnt/
fi

# The maas needs boot from EFI/UBUNTU/shimx64.efi, but recovery doesn't
# Here to create an EFI/UBUNTU/ boot assets
sudo mkdir img_mnt/EFI/UBUNTU/
sudo cp img_mnt/EFI/BOOT/* img_mnt/EFI/UBUNTU/
if [ ! -f /usr/lib/shim/shimx64.efi.signed ];then
    sudo apt update
    sudo apt install -y shim-signed
fi
sudo cp /usr/lib/shim/shimx64.efi.signed img_mnt/EFI/UBUNTU/

sudo umount img_mnt
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
