#!/bin/sh
# This is the hook for snapcraft install of recovery part

mkdir -p $SNAPCRAFT_PART_INSTALL/recovery-assets
cp -r recovery-includes/recovery $SNAPCRAFT_PART_INSTALL/recovery-assets/
cp -r initrd-hooks $SNAPCRAFT_PART_INSTALL/recovery-assets/
# XXX: it seems the config.yaml has been copied by snapcraft.yaml. To remove it.
cp ../../recovery-configs/src/config.yaml $SNAPCRAFT_PART_INSTALL/recovery-assets/recovery/
cp -r writable-includes $SNAPCRAFT_PART_INSTALL/
cp -r ubuntu-image-hooks $SNAPCRAFT_PART_INSTALL/
