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
  sleep 1
done

# run hooks
hookdir=/writable/system-data/var/lib/devmode-firstboot/hooks.d/
for hook in $hookdir/*
do 
    bash $hook
done

# umount, inactive snappy_boot_entry and reboot
touch /writable/system-data/var/lib/devmode-firstboot/devmode-firstboot.stamp

# re-enable login prompt
systemctl enable getty@tty1.service
systemctl enable serial-getty@ttyS0.service

reboot
