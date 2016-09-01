#!/bin/sh

set -x
set -e

for SNAPS in `find /writable/system-data/var/lib/oem/snaps-devmode/ -name "*.snap" -type f` ; do
	snap install --devmode "$SNAPS"
done

for SNAPS in `find /writable/system-data/var/lib/oem/snaps/ -name "*.snap" -type f` ; do
	snap install "$SNAPS"
done

touch /writable/system-data/var/lib/devmode-firstboot/devmode-firstboot.stamp
