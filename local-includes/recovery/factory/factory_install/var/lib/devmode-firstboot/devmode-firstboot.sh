#!/bin/sh

set -x
set -e

# import variable RECOVERYFSLABEL, RECOVERY_TYPE
. /writable/system-data/var/lib/devmode-firstboot/conf.sh

cd /writable/system-data/var/lib/devmode-firstboot

#FIXME: when checkbox snap for arm ready
#for SNAPS in `find . -name "*.snap" -type f` ; do
#	snap install --devmode "$SNAPS"
#done

#for SNAPS in `find /writable/system-data/var/lib/oem/snaps-devmode/ -name "*.snap" -type f` ; do
#	snap install --devmode "$SNAPS"
#done

#for SNAPS in `find /writable/system-data/var/lib/oem/snaps/ -name "*.snap" -type f` ; do
#	snap install "$SNAPS"
#done

count=0
while true; do
	if [ -f "/writable/factory-diag-result" ]; then
		clear
		result=$(cat /writable/factory-diag-result)
		printf "\nfactory-diag result: %s \n" "$result"
		sleep 3
		break
	else
		i=0
		max=$((count % 4))
		while [ "$i" != "$max" ]; do
			printf "%s Running factory diag, please wait\n" "$(date)"
			i=$((i + 1))
		done
		count=$((count + 1))
		sleep 1
	fi
done

# find and mount recovery partition
recovery_dev=$(findfs LABEL=$RECOVERYFSLABEL)
recovery_dir=$(mktemp -d)
mount "$recovery_dev" "$recovery_dir"

# copy factory-diag logs to recovery partition
rm -rf "$recovery_dir"/factory-log/
mkdir -p "$recovery_dir"/factory-log/
cp -r /root/.local/share/checkbox-ng "$recovery_dir"/factory-log/
cp /writable/factory-diag-result "$recovery_dir"/factory-log/
umount "$recovery_dir"

# remove factory-diag snap
#echo "Remove factory-diag"
#snap remove factory-diag
# FIXME: when checkbox snap for pi3 ready

# umount, inactive snappy_boot_entry and reboot
grub-editenv /boot/efi/EFI/ubuntu/grub/grubenv unset cloud_init_disabled
touch /writable/system-data/var/lib/devmode-firstboot/devmode-firstboot.stamp

reboot
