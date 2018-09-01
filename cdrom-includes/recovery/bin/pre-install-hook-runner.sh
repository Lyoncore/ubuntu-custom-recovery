#!/bin/bash -x
exec &> >(tee -a "/var/log/recovery/pre-install-hook-runner.log")
CDROM_MNT="/cdrom"
RECO_MNT="/run/recovery"

[ -d $RECO_MNT ] || mkdir -p $RECO_MNT

mount --bind $CDROM_MNT $RECO_MNT

hookdir=$(awk -F ": " '/oem-preinst-hook-dir/{print $2 }' $RECO_MNT/recovery/config.yaml)
if [ ! -z $hookdir ]; then
    OEM_PREINST_HOOK_DIR=$RECO_MNT/recovery/factory/$hookdir
fi

# The preinstall hook not needed in headless_installer
if [ -d $OEM_PREINST_HOOK_DIR ]; then
    echo "[Factory Install Preinstall hook] Run scripts in $OEM_PREINST_HOOK_DIR"
    export RECOVERYTYPE=$recoverytype
    export RECOVERYMNT=$RECO_MNT
    find "$OEM_PREINST_HOOK_DIR" -type f ! -name ".gitkeep" | sort | while read -r filename;
    do
        bash "$filename" 2>&1 | (tee -a /var/log/recovery/preinstall_hooks.log &)
        ret=${PIPESTATUS[0]}
        if [ $ret -ne 0 ];then
            echo "Hook return error in $filename , return=$ret" >> /var/log/recovery/preinstall_hooks.err
        fi
        echo "\n" >> /var/log/recovery/preinstall_hooks.log
    done
fi

umount $RECO_MNT
