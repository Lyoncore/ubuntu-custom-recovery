#!/bin/sh
# This is the hook for snapcraft install of recovery part

mkdir -p $SNAPCRAFT_PART_INSTALL/recovery-assets
cp -r recovery-includes/recovery $SNAPCRAFT_PART_INSTALL/recovery-assets/
cp -r initrd-hooks $SNAPCRAFT_PART_INSTALL/recovery-assets/
cp -r writable-includes $SNAPCRAFT_PART_INSTALL/
cp -r ubuntu-image-hooks $SNAPCRAFT_PART_INSTALL/
