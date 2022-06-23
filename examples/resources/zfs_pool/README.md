# Create a zpool with a few custom properties set.
Create a striped mirrored zpool spanning (sda, sdb) and (sdc, sdd), with a few custom properties set

This is roughly equivalent of the following command:
`zpool create -o relatime=on -o recordsize=4K -o compression=on mirror /dev/sda /dev/sdb mirror /dev/sdc /dev/sdd`