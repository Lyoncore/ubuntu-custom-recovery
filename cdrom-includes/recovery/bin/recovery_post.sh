#!/bin/bash -x
exec &> >(tee -a "/var/log/recovery/recovery_post.log")
RECOVERY_ENTRY="factory_restore"
OS_ENTRY="ubuntu"
ROOTFSMNT="/target"
BOOTMNT="/target/boot/efi"
RECO_MNT="/run/recovery"

update_grubenv() {
    mount -o remount,rw /cdrom
    grub-editenv /cdrom/boot/grub/grubenv set recovery_type=factory_restore
    mount -o remount,ro /cdrom
}

del_old_boot_entries() {
    recovery_entry_nr=$(efibootmgr | grep $RECOVERY_ENTRY | cut -d " " -f 1 | tr -d Boot | tr -d "*")
    if [ -n "$recovery_entry_nr" ]; then
        for nr in $recovery_entry_nr
        do
            efibootmgr -b $nr -B
        done
    fi

    os_entry_nr=$(efibootmgr | grep $OS_ENTRY | cut -d " " -f 1 | tr -d Boot | tr -d "*")
    if [ -n "$os_entry_nr" ];then
        for nr in $os_entry_nr
        do
            efibootmgr -b $nr -B
        done
    fi
}

rebuild_boot_entries() {
    recovery=$(mount | grep cdrom | cut -d " " -f 1) # it would find cdrom mount
    if [[ $recovery = *"mmcblk"* ]]; then
        recovery_dev=${recovery::-2}
    else
        recovery_dev=${recovery::-1}
    fi

    recovery_part=${recovery: -1}
    efibootmgr -c -d $recovery_dev -p $recovery_part -l "\\EFI\\BOOT\\BOOTX64.EFI" -L $RECOVERY_ENTRY

    boot=$(mount | grep boot | cut -d " " -f 1)  # it would find boot/efi mount
    if [[ $boot = *"mmcblk"* ]]; then
        boot_dev=${boot::-2}
    else
        boot_dev=${boot::-1}
    fi

    boot_part=${boot: -1}
    efibootmgr -c -d $boot_dev -p $boot_part -l "\\EFI\\ubuntu\\shimx64.efi" -L $OS_ENTRY
}

set_next_bootentry() {
    os_entry_nr=$(efibootmgr | grep $OS_ENTRY | cut -d " " -f 1 | tr -d Boot | tr -d "*")
    if [ -n "$os_entry_nr" ];then
        efibootmgr -n $os_entry_nr
    fi
}

chroot_cmd() {
    root=$1
    cmd=$2
    mount --bind /sys $root/sys
    mount --bind /proc $root/proc
    mount --bind /dev $root/dev
    mount --bind /run $root/run

    chroot $root $cmd 2>&1

    umount $root/sys
    umount $root/proc
    umount $root/dev
    umount $root/run
}

update_grub_menu() {
    LABEL=$(awk -F ": " '/filesystem-label/{print $2 }' $RECO_MNT/recovery/config.yaml)
    if [ ! -n "$LABEL"]; then
        exit 1
    fi

    cat << EOF >> $ROOTFSMNT/etc/grub.d/40_custom
menuentry "Factory Restore" {
        set gfxpayload=keep
        # load recovery system
        echo "[grub.cfg] load factory_restore system"
        search --no-floppy --set --label "$LABEL"
        echo "[grub.cfg] root: \${root}"
        set cmdline="file=/cdrom/preseed/oem-ubuntu-server.seed boot=casper union=aufs console=tty1 panic=-1 fixrtc -- recoverytype=factory_restore recoverylabel=$LABEL recoveryos=ubuntu_classic_curtin"
        echo "[grub.cfg] loading kernel..."
        linux (\$root)/casper/vmlinuz \$cmdline
        echo "[grub.cfg] loading initrd..."
        initrd (\$root)/casper/initrd.lz
        echo "[grub.cfg] boot..."
        boot
}
EOF

    chroot_cmd $ROOTFSMNT update-grub
}

move_log_to_rootfs() {
    mkdir $ROOTFSMNT/var/log/recovery/
    cp -r /var/log/recovery/* $ROOTFSMNT/var/log/recovery/
}



apt install -y efibootmgr
update_grubenv
del_old_boot_entries
rebuild_boot_entries
set_next_bootentry
update_grub_menu

# Check the recovery type
for x in $(cat /proc/cmdline); do
    case ${x} in
        recoverytype=*)
            recoverytype=${x#*=}
        ;;
        recoveryos=*)
            recoveryos=${x#*=}
        ;;
     esac
done

# execute the hooks
hookdir=$(awk -F ": " '/oem-postinst-hook-dir/{print $2 }' $RECO_MNT/recovery/config.yaml)
if [ ! -z $hookdir ]; then
    OEM_POSTINST_HOOK_DIR=$RECO_MNT/recovery/factory/$hookdir
fi

# The factory_restore posthook not needed in headless_installer
if [ ! -z $recoverytype ] && [ $recoverytype != "headless_installer" ]; then
    if [ -d $OEM_POSTINST_HOOK_DIR ]; then
        echo "[Factory Restore Posthook] Run scripts in $OEM_POSTINST_HOOK_DIR"
        export RECOVERYTYPE=$recoverytype
        export RECOVERYMNT=$RECO_MNT
        find "$OEM_POSTINST_HOOK_DIR" -type f | sort | while read -r filename; do bash "$filename"; done
    fi
fi

move_log_to_rootfs

$RECO_MNT/recovery/bin/pre-reboot-hook-runner.sh
