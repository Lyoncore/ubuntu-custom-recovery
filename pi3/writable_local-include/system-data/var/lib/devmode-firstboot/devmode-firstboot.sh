#!/bin/sh

set -x
set -e

mkdir -p /writable/system-data/var/log/recovery/
exec &> >(tee -a "/writable/system-data/var/log/recovery/devmode-firstboot.log")
echo "[Factory Restore Process] Start devmode-firstboot $(date -Iseconds --utc)"

# disable login prompt temporarily
systemctl stop getty@tty1.service
systemctl disable getty@tty1.service
systemctl stop serial-getty@ttyS0.service
systemctl disable serial-getty@ttyS0.service
systemctl daemon-reload

# Wait for firstboot finish and start factory diag
while true ; do
  if [ "$(snap changes | grep 'Initialize system state' | grep Done)" ] ; then
     break
  fi
  echo "Initialize system state is in progress, waiting..."
  ### DEBUG only
  #snap changes
  #PS1='debugshell> ' /bin/sh -i </dev/console >/dev/console 2>&1
  sleep 1
done

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

# re-enable login prompt
systemctl enable getty@tty1.service
systemctl enable serial-getty@ttyS0.service

reboot
