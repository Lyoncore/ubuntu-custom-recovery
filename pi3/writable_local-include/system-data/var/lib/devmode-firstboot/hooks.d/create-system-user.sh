#!/bin/bash

set -e
set -x
exec &> >(tee -a "/writable/system-data/var/log/recovery/devmode-firstboot.log" "/run/recovery/MFGMEDIA/fist.log")

if [ "$(snap managed)" = "true" ]; then
	echo "System already managed, exiting"
	exit 0
fi

while ! snap changes ; do
	echo "No changes yet, waiting"
	sleep 1
done

while snap changes | grep -E '(Do|Doing) .*Initialize system state' ;  do
	echo "Initialize system state is in progress, waiting"
	sleep 1
done

if [ "$(snap known system-user)" != "" ]; then
	echo "Trying to create known user"
	snap create-user --known --sudoer || true
fi
