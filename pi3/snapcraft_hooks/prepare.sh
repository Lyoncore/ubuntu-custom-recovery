#!/bin/sh
# This is the hook for snapcraft prepare of recovery part

cp ../../../uboot.env.in pi3/local-includes/
LABEL=$(cat pi3/config.yaml | grep filesystem-label | awk -F ': ' '{print $2}')
sed -i "s/LABEL=/LABEL=$LABEL/" pi3/local-includes/uboot.env.in
