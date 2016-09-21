#!/bin/sh

if [ $RECOVERY_TYPE != "factory_install" ]; then
    return
fi

# install factory-diag snap
#snap install --devmode /writable/system-data/var/lib/devmode-firstboot/factory-diag*.snap

#while true; do
#	if [ -f "/writable/factory-diag-result" ]; then
#		clear
#		result=$(cat /writable/factory-diag-result)
#		printf "\nfactory-diag result: %s \n" "$result"
#		break
#	fi
	printf "%s Running factory diag, please wait\n" "$(date)"
	sleep 3
#done

# find and mount recovery partition
recovery_dev=$(findfs LABEL=$RECOVERYFSLABEL)
recovery_dir=$(mktemp -d)
mount "$recovery_dev" "$recovery_dir"

# copy factory-diag logs to recovery partition
dst="$recovery_dir"/factory-log/
rm -rf $dst
mkdir -p $dst
#cp -a /root/snap/factory-diag/x1/.local/share/checkbox-ng $dst
#cp /writable/factory-diag-result $dst
umount "$recovery_dir"
rmdir "$recovery_dir"

# remove factory-diag snap
echo "Remove factory-diag"
#snap remove factory-diag

