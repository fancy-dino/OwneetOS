#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# archiso profile for the OwneetOS ISO. Derived from archiso's "baseline" profile (archiso 91).
# Build it inside the builder VM: tools/build-in-vm tools/build-iso
# shellcheck disable=SC2034

iso_name="owneetos"
iso_label="OWNEET_$(date --date="@${SOURCE_DATE_EPOCH:-$(date +%s)}" +%Y%m)"
iso_publisher="OwneetOS <https://github.com/fancy-dino/OwneetOS>"
iso_application="OwneetOS"
iso_version="$(date --date="@${SOURCE_DATE_EPOCH:-$(date +%s)}" +%Y.%m.%d)"
install_dir="owneet"
# UEFI only, booted by systemd-boot (PROJECT_RULES.md section 4).
bootmodes=('uefi.systemd-boot')
pacman_conf="pacman.conf"
airootfs_image_type="erofs"
airootfs_image_tool_options=('-zlzma,109' -E 'ztailpacking')
file_permissions=(
  ["/etc/shadow"]="0:0:400"
)
