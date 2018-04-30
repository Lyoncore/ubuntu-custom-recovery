#!/bin/bash

print_help()
{
    echo "usage: $0 --u-c-r=<path to ubuntu-custom-recovery> --assets-src=<path to recovery assets> --oem-cdimage-script=<path to oem-cdimage-script> --oem-livecd-rootfs=<path to oem-livecd-rootfs> [--hook=<path to post hook>]"
}

# Handle commandline parameters
while [ -n "$1" ]; do
    case "$1" in
        --assets-src=*)
            ASSETS_SRC=${1#*=}
            ;;
        --oem-cdimage-script=*)
            CDIMAGE_SCRIPT=${1#*=}
            ;;
        --oem-livecd-rootfs=*)
            LIVECD_ROOTFS=${1#*=}
            ;;
        --u-c-r=*)
            U_C_R=${1#*=}
            ;;
        --hook=*)
            HOOK=${1#*=}
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

if [ -z $ASSETS_SRC ] || [ -z $CDIMAGE_SCRIPT ] || [ -z $LIVECD_ROOTFS ] || [ -z $U_C_R ]; then
    print_help
    exit 1
fi

if [ ! -d $U_C_R ]; then
	echo "ubuntu-custom-recovery custom recovery dir not found ($U_C_R)"
	exit 1
fi
if [ ! -d $CDIMAGE_SCRIPT ]; then
	echo "oem-cdimage-script dir not found ($CDIMAGE_SCRIPT)"
	exit 1
fi

if [ ! -d $LIVECD_ROOTFS ]; then
	echo "oem-livecd-rootfs dir not found ($LIVECD_ROOTFS)"
	exit 1
fi

if [ ! -d $ASSETS_SRC ]; then
	echo "recovery assets source dir not found ($ASSETS_SRC)"
	exit 1
fi

if [ ! -f $U_C_R/recovery-includes/recovery/bin/recovery.bin ]; then
    echo "$U_C_R/recovery-includes/recovery/bin/recovery.bin not found. not compiled yet?"
    exit 1
fi

# clean old recovery if presents
if [ -d $CDIMAGE_SCRIPT/recovery ]; then
    rm -rf $CDIMAGE_SCRIPT/recovery
fi

#copy the cdrom includes
cp -r $U_C_R/cdrom-includes/recovery/ $CDIMAGE_SCRIPT/
cp $U_C_R/recovery-includes/recovery/bin/recovery.bin $CDIMAGE_SCRIPT/recovery/bin/
cp -r $ASSETS_SRC/* $CDIMAGE_SCRIPT/recovery

#copy grub config files
rm -rf $CDIMAGE_SCRIPT/boot/*
cp -r $U_C_R/grub-includes/boot/* $CDIMAGE_SCRIPT/boot/

#copy initrd files
cp $U_C_R/initrd-casper-hooks/scripts/casper-bottom/99ubuntu_custom-recovery $LIVECD_ROOTFS/live-build/ubuntu-server/includes.binary/
cp $U_C_R/initrd-casper-hooks/live-build/ubuntu-server/hooks/033-ubuntu-custom-recovery.binary $LIVECD_ROOTFS/live-build/ubuntu-server/hooks/

if [ -f $HOOK ];then
    $HOOK
fi
