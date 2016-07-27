#!/bin/sh

set -x

cd /writable/system-data/var/lib/devmode-firstboot

for DEVMODESNAPS in *.snap ; do
	snap install --devmode $DEVMODESNAPS
done

touch /writable/system-data/var/lib/devmode-firstboot/devmode-firstboot.stamp
