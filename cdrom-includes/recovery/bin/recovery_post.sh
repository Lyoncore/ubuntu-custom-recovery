#!/bin/bash -x
RECOVERY_ENTRY="factory_restore"
OS_ENTRY="ubuntu"

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
    recovery_dev=$(echo $recovery | sed 's/.$//')
    recovery_part=$(echo $recovery | tr -d $recovery_dev)
    efibootmgr -c -d $recovery_dev -p $recovery_part -l "\\EFI\\BOOT\\BOOTX64.EFI" -L $RECOVERY_ENTRY

    boot=$(mount | grep boot | cut -d " " -f 1)  # it would find boot/efi mount
    boot_dev=$(echo $boot | sed 's/.$//')
    boot_part=$(echo $boot | tr -d $boot_dev)
    efibootmgr -c -d $boot_dev -p $boot_part -l "\\EFI\\ubuntu\\shimx64.efi" -L $OS_ENTRY
}

set_next_bootentry() {
    os_entry_nr=$(efibootmgr | grep $OS_ENTRY | cut -d " " -f 1 | tr -d Boot | tr -d "*")
    if [ -n "$os_entry_nr" ];then
        efibootmgr -n $os_entry_nr
    fi
}

apt install -y efibootmgr
del_old_boot_entries
rebuild_boot_entries
set_next_bootentry
