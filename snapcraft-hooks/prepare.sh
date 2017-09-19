#!/bin/sh
# This is the hook for snapcraft prepare of recovery part

cp ../../../uboot.env.in recovery-includes/
LABEL=$(cat ../../recovery-configs/src/config.yaml | grep filesystem-label | awk -F ': ' '{print $2}')
sed -i "s/LABEL=/LABEL=$LABEL/" recovery-includes/uboot.env.in
