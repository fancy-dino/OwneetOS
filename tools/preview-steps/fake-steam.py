#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Writes an invented Steam folder for the frontend preview ([[step-4.2c-steam-without-its-interface]]):
# the account's licences (appcache/packageinfo.vdf, binary version 28), the apps' types and names
# (appcache/appinfo.vdf, binary version 29 with its table of keys), a few installed games, a game
# on a disk that is not connected, and a signed-in account. Names and ids are invented.
#
#   fake-steam.py STEAM_DIR
import os
import struct
import sys

# app id → (name, type, installed)
APPS = {
    2001: ("Glass Canyon", "game", True),
    2002: ("Paper Comets", "game", True),
    2003: ("Coral Line", "game", False),     # installed on a disk that is not connected
    2004: ("Iron Tundra", "game", False),
    2005: ("Feather Fall", "game", False),
    2006: ("Deep Lantern", "game", False),
    2007: ("Marble Sky", "game", False),
    2008: ("Echo Valley", "game", False),
    2009: ("Frost Relay", "game", False),
    2010: ("Sunken Bell", "game", True),
    2011: ("Orchard", "game", False),
    2012: ("Brass Comet", "game", False),
    2101: ("Glass Canyon Soundtrack", "music", False),
    2102: ("Paper Comets: Extra Levels", "dlc", False),
    2103: ("Iron Tundra Demo", "demo", False),
}
# packages: id → (status, app ids); package 0 is everyone's and left out by the console
PACKAGES = {
    0: (0, [7, 2001]),
    3001: (0, [2001, 2101]),
    3002: (0, [2002, 2102]),
    3003: (0, [2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010, 2011, 2012, 2103]),
    3004: (2, [2099]),                         # an inactive licence: not owned
}


def kv_text(value, keys=None):
    """Binary KeyValues of a dict: strings, ints, nested dicts; keys as C strings or table indices."""
    out = b""
    for k, v in value.items():
        if isinstance(v, dict):
            t, data = 0, kv_text(v, keys) + b"\x08"
        elif isinstance(v, int):
            t, data = 2, struct.pack("<i", v)
        else:
            t, data = 1, v.encode() + b"\0"
        if keys is None:
            name = k.encode() + b"\0"
        else:
            if k not in keys:
                keys.append(k)
            name = struct.pack("<i", keys.index(k))
        out += bytes([t]) + name + data
    return out


def packageinfo():
    out = struct.pack("<II", 0x06565528, 1)
    for pid, (status, apps) in PACKAGES.items():
        body = {str(pid): {"packageid": pid, "status": status, "appids": {str(i): a for i, a in enumerate(apps)}}}
        out += struct.pack("<I", pid) + b"\0" * 20 + struct.pack("<I", 1) + struct.pack("<Q", 0)
        out += kv_text(body) + b"\x08"
    return out + struct.pack("<I", 0xFFFFFFFF)


def appinfo():
    keys = []
    entries = b""
    for appid, (name, kind, _) in APPS.items():
        body = kv_text({"appinfo": {"appid": appid, "common": {"name": name, "type": kind.capitalize()}}}, keys) + b"\x08"
        head = struct.pack("<IIQ", 2, 0, 0) + b"\0" * 20 + struct.pack("<I", 1) + b"\0" * 20
        entries += struct.pack("<II", appid, len(head) + len(body)) + head + body
    entries += struct.pack("<I", 0)
    start = 8 + 8                               # magic, universe, offset of the table of keys
    table = struct.pack("<I", len(keys)) + b"".join(k.encode() + b"\0" for k in keys)
    return struct.pack("<IIq", 0x07564429, 1, start + len(entries)) + entries + table


def main():
    steam = sys.argv[1]
    for d in ("appcache", "steamapps", "config", "ubuntu12_32"):
        os.makedirs(os.path.join(steam, d), exist_ok=True)
    with open(os.path.join(steam, "appcache", "packageinfo.vdf"), "wb") as f:
        f.write(packageinfo())
    with open(os.path.join(steam, "appcache", "appinfo.vdf"), "wb") as f:
        f.write(appinfo())
    with open(os.path.join(steam, "steamapps", "libraryfolders.vdf"), "w") as f:
        f.write('"libraryfolders"\n{\n\t"0"\n\t{\n\t\t"path"\t\t"%s"\n\t\t"apps"\n\t\t{\n%s\t\t}\n\t}\n'
                '\t"1"\n\t{\n\t\t"path"\t\t"/run/media/owneet/Games SSD/SteamLibrary"\n\t\t"label"\t\t""\n'
                '\t\t"apps"\n\t\t{\n\t\t\t"2003"\t\t"9000000000"\n\t\t}\n\t}\n}\n'
                % (steam, "".join('\t\t\t"%d"\t\t"1000"\n' % a for a, v in APPS.items() if v[2])))
    for appid, (name, _, installed) in APPS.items():
        if installed:
            with open(os.path.join(steam, "steamapps", "appmanifest_%d.acf" % appid), "w") as f:
                f.write('"AppState"\n{\n\t"appid"\t\t"%d"\n\t"name"\t\t"%s"\n\t"StateFlags"\t\t"4"\n}\n' % (appid, name))
    with open(os.path.join(steam, "config", "loginusers.vdf"), "w") as f:
        f.write('"users"\n{\n\t"76561190000000001"\n\t{\n\t\t"AccountName"\t\t"preview"\n\t\t"MostRecent"\t\t"1"\n\t}\n}\n')
    open(os.path.join(steam, "ubuntu12_32", "steam"), "w").close()


if __name__ == "__main__":
    main()
