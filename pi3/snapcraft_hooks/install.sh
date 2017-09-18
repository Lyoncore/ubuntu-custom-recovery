#!/bin/sh
# This is the hook for snapcraft install of recovery part

mkdir -p $SNAPCRAFT_PART_INSTALL/recovery-assets
cp -r pi3/local-includes/recovery $SNAPCRAFT_PART_INSTALL/recovery-assets/
cp -r pi3/initrd_local-includes $SNAPCRAFT_PART_INSTALL/recovery-assets/
cp pi3/config.yaml $SNAPCRAFT_PART_INSTALL/recovery-assets/recovery/
mksquashfs pi3/writable_local-include $SNAPCRAFT_PART_INSTALL/recovery-assets/recovery/writable_local-include.squashfs -all-root
mkenvimage -r -s 131072 -o $SNAPCRAFT_PART_INSTALL/uboot.env pi3/local-includes/uboot.env.in
