#!/bin/sh

set -x

RECOVERYFSLABEL="recovery"

cd /writable/system-data/var/lib/devmode-firstboot

for DEVMODESNAPS in *.snap ; do
	snap install --devmode $DEVMODESNAPS
done

count=0
while true; do
	if [ -f "/writable/factory-diag-result" ]; then
		clear
		result=$(cat /writable/factory-diag-result)
		printf "\nfactory-diag result: $result \n"
		sleep 3
		break
	else
		i=0
		max=$(($count % 4))
		while [ "$i" != "$max" ]; do
			printf "`date` Running factory diag, please wait\n"
			i=$(($i + 1))
		done
		count=$(($count + 1))
		sleep 1
	fi
done

# find and mount recovery partition
recovery_dev=$(findfs LABEL=$RECOVERYFSLABEL)
recovery_dir=$(mktemp -d)
mount $recovery_dev $recovery_dir

# copy serial, gpg key and logs
rm -rf $recovery_dir/factory-log/
mkdir -p $recovery_dir/factory-log/
cp -r /root/.local/share/checkbox-ng $recovery_dir/factory-log/
cp /writable/factory-diag-result $recovery_dir/factory-log/

# umount, inactive snappy_boot_entry and reboot
umount $recovery_dir

# remove factory-diag snap
echo "Remove factory-diag"
snap remove factory-diag

grub-editenv /boot/efi/EFI/ubuntu/grub/grubenv unset cloud_init_disabled
touch /writable/system-data/var/lib/devmode-firstboot/devmode-firstboot.stamp
