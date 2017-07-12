#!/bin/sh

set -x
set -e

# import variable RECOVERYFSLABEL, RECOVERY_TYPE
. /writable/system-data/var/lib/devmode-firstboot/conf.sh

#find /writable/system-data/var/lib/oem/snaps-devmode/ -name "*.snap" -type f -exec snap install --devmode {} \;
#find /writable/system-data/var/lib/oem/snaps/ -name "*.snap" -type f -exec snap install {} \;

# run hooks
hookdir=/writable/system-data/var/lib/devmode-firstboot/hooks.d/
. $hookdir/ORDER

# umount, inactive snappy_boot_entry and reboot
#grub-editenv /boot/efi/EFI/ubuntu/grub/grubenv unset cloud_init_disabled
touch /writable/system-data/var/lib/devmode-firstboot/devmode-firstboot.stamp

reboot
