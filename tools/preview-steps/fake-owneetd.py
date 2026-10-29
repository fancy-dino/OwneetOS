#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
#
# A pretend owneetd for tools/frontend-preview (steps files starting with "@owneetd"): the same
# HTTP + JSON API on a Unix socket, with invented networks, controllers and Bluetooth devices,
# so that the Settings panels and windows can be seen without Wi-Fi or Bluetooth hardware.
# It is not the daemon: only what the interface asks, and simple made-up behaviour:
#   - Wi-Fi connect: password "wrongpassword" → wrong_password, other valid ones → connected (1.5 s);
#   - auto-pair on: after 3 s a "DualSense Wireless Controller" pairs;
#   - scan: wifi.scan_done after 1.5 s;
#   - display: two screens, modes of the TV, brightness; a change waits for "Keep" (screen 30 s,
#     mode 15 s);
#   - optional web apps (5.0.0): none installed; the first install takes 3 s (the engine), the next
#     ones and the removals are immediate; the last removal takes 1.5 s;
#   - the time zone (6.1): set at once; an Xbox wireless adapter that needs its firmware, which
#     installs in 3 s (FAKE_FIRSTSTART=1: a new console, not connected to any network).
#
# Steps can send it events (tools/frontend-preview "event TYPE JSON"): one line each in the FIFO
# given as second argument.
#
#   fake-owneetd.py SOCKET [EVENTS_FIFO]

import datetime
import json
import os
import queue
import re
import shutil
import socketserver
import sys
import threading
import time
import urllib.parse
import zipfile
import zoneinfo
from http.server import BaseHTTPRequestHandler

lock = threading.Lock()
listeners = []

state = {
    "time": {"zone": "UTC", "synchronized": True},
    "xone": {"needed": True, "state": ""},
    "network": {
        "available": True, "state": "connected", "connectivity": "full",
        "wifi": {"present": True, "enabled": True, "hardware_enabled": True, "state": "connected",
                 "ssid": "Casa Rossi", "strength": 82},
        "ethernet": {"present": True, "connected": False},
    },
    "networks": [
        {"ssid": "Casa Rossi", "strength": 82, "security": "wpa", "known": True, "connected": True},
        {"ssid": "Casa Rossi 5G", "strength": 64, "security": "wpa", "known": True, "connected": False},
        {"ssid": "Cafe Wi-Fi", "strength": 47, "security": "wpa", "known": False, "connected": False},
        {"ssid": "Biblioteca", "strength": 30, "security": "open", "known": False, "connected": False},
        {"ssid": "Office", "strength": 25, "security": "enterprise", "known": False, "connected": False},
        {"ssid": "TIM-38211", "strength": 21, "security": "wpa3", "known": False, "connected": False},
    ],
    "controllers": [
        {"id": "aa:bb:cc:dd:ee:01", "name": "Xbox Wireless Controller", "brand": "xbox", "connection": "bluetooth",
         "vendor_id": "045e", "product_id": "0b13", "has_guide": True, "battery": 78},
        {"id": "aa:bb:cc:dd:ee:02", "name": "Pro Controller", "brand": "nintendo", "connection": "bluetooth",
         "vendor_id": "057e", "product_id": "2009", "has_guide": True, "battery_level": "normal"},
        {"id": "usb-0000:00:14.0-2/input0", "name": "8BitDo Ultimate", "brand": "8bitdo", "connection": "usb",
         "vendor_id": "2dc8", "product_id": "3106", "has_guide": True},
    ],
    "bluetooth": {
        "adapter": True, "powered": True, "discovering": False, "auto_pair": False,
        "devices": [
            {"address": "AA:BB:CC:DD:EE:01", "name": "Xbox Wireless Controller", "paired": True, "trusted": True,
             "connected": True, "gamepad": True, "battery": 78},
            {"address": "AA:BB:CC:DD:EE:02", "name": "Pro Controller", "paired": True, "trusted": True,
             "connected": True, "gamepad": True},
            {"address": "AA:BB:CC:DD:EE:03", "name": "DualSense Wireless Controller", "paired": True, "trusted": True,
             "connected": False, "gamepad": True},
            {"address": "AA:BB:CC:DD:EE:04", "name": "JBL Tune 510BT", "paired": True, "trusted": True,
             "connected": False, "gamepad": False},
        ],
    },
    "display": {
        "available": True,
        "screens": [
            {"id": "card0-HDMI-A-1", "connector": "HDMI-A-1", "name": "LG TV", "internal": False, "active": True},
            {"id": "card1-DP-2", "connector": "DP-2", "name": "DELL P2419H", "internal": False, "active": False},
        ],
        "modes": ["3840x2160@60", "3840x2160@30", "2560x1440@120", "2560x1440@60", "1920x1080@120",
                  "1920x1080@60", "1280x720@60"],
        "mode": "auto",
        "brightness": 80,
    },
    "apps": [],
    # Steam (4.2): change it with the event line "fake.steam {...}" (e.g. {"allowed": false})
    # testers' updates from a USB stick (4.2b)
    "update": {"tester": True, "state": "", "packages": [], "running": False},
    # Windows games (4.5): nothing until the interface says the library has one; then about
    # 20 seconds of "getting ready"
    "windows": {"state": "none", "percent": 0, "needed": False},
    # games installed from their installers (4.6): queued or in progress
    "installs": [],
    # the controller as a mouse (4.7)
    "pointer": {"on": False},
    "steam": {"installed": True, "allowed": True, "prepared": False, "running": False, "games": [],
              "downloads": [], "away": {}, "account": False, "licences": 0, "nvidia": False},
    # optional web apps (5.0.0)
    "catalog": {"allowed": True, "apps": [
        {"id": i, "nameKey": "app." + i, "icon": i[0].upper(), "url": "", "installed": False, "sizeMiB": 500}
        for i in ("netflix", "youtube", "spotify")]},
    "audio": {"available": True, "output": "hdmi", "volume": 60, "muted": False,
              "outputs": [{"id": "hdmi", "name": "TV (HDMI)"}, {"id": "speakers", "name": "Speakers"}]},
}


def publish(kind, data=None):
    with lock:
        for q in listeners:
            q.put((kind, data))


def later(seconds, fn):
    threading.Timer(seconds, fn).start()


# Steam's downloads (4.2c): the first one moves, the others wait; about 20 seconds a game
def steam_publish():
    with lock:
        data = json.loads(json.dumps(state["steam"]))
    publish("steam.changed", data)


def steam_install(appid):
    with lock:
        ds = state["steam"]["downloads"]
        if any(d["appid"] == appid for d in ds):
            return
        ds.append({"appid": appid, "state": "queued", "done": 0, "total": 4_200_000_000, "rate": 0})
        start = len(ds) == 1
    steam_publish()
    if start:
        later(0.5, steam_tick)


def steam_tick():
    with lock:
        ds = state["steam"]["downloads"]
        if not ds:
            return
        d = ds[0]
        d["state"], d["rate"] = "downloading", 41_600_000
        d["done"] = min(d["total"], d["done"] + d["total"] // 40)
        if d["done"] >= d["total"]:
            ds.pop(0)
            state["steam"]["games"].append(d["appid"])
    steam_publish()
    later(0.5, steam_tick)


# Installers (4.6): an install takes a few seconds (GOG's .sh with a percentage, Windows
# installers without), then the game is in the home folder's games folder with owneetd's mark
def install_inspect(body):
    path = body.get("installer", "")
    info = {"need": 0, "exact": False, "free": 212 * 10**9, "licence": ""}
    if body.get("kind") == "sh":
        try:
            with zipfile.ZipFile(path) as z:
                info["need"] = sum(f.file_size for f in z.infolist() if f.filename.startswith("data/noarch/"))
                info["exact"] = True
                for f in z.infolist():
                    if re.search(r"(?i)(eula|licen[cs]e|terms)[^/]*\.txt$", f.filename):
                        info["licence"] = z.read(f).decode("utf-8", "replace")
                        break
        except (OSError, zipfile.BadZipFile):
            pass
    else:
        try:
            info["need"] = 2 * os.path.getsize(path)
        except OSError:
            pass
    return info


def installs_copy():
    return {"installs": json.loads(json.dumps(state["installs"]))}


def install_tick():
    with lock:
        cur = next((i for i in state["installs"] if i["state"] == "installing"), None)
        if cur is None:
            return
        cur["done"] += cur["total"] // 6 if cur["total"] else 1
        finished = cur["done"] >= (cur["total"] or 30)   # Windows installers: half a minute
        if finished:
            state["installs"].remove(cur)
            nxt = next((i for i in state["installs"] if i["state"] == "queued"), None)
            if nxt:
                nxt["state"] = "installing"
        snapshot = installs_copy()
        more = bool(state["installs"])
    if finished:
        dest = cur["dest"]
        os.makedirs(dest, exist_ok=True)
        start = os.path.join(dest, "start.sh" if cur["kind"] == "sh" else "game.exe")
        with open(start, "w") as f:
            f.write("#!/bin/sh\nexec true\n")
        os.chmod(start, 0o755)
        with open(os.path.join(dest, ".owneet-installed"), "w") as f:
            json.dump({"installer": cur["installer"], "kind": cur["kind"]}, f)
    publish("installs.changed", snapshot)
    if finished:
        publish("install.done", {"dest": cur["dest"], "title": cur["title"]})
    if more:
        later(1, install_tick)


def windows_tick():
    with lock:
        w = state["windows"]
        w["percent"] = min(100, w["percent"] + 5)
        if w["percent"] >= 100:
            w["state"] = "ready"
        data = dict(w)
    publish("windows.changed", data)
    if data["state"] != "ready":
        later(1, windows_tick)


def steam_uninstall(appid):
    with lock:
        st = state["steam"]
        st["downloads"] = [d for d in st["downloads"] if d["appid"] != appid]
        st["games"] = [g for g in st["games"] if g != appid]
    steam_publish()


def set_connected(ssid):
    with lock:
        for n in state["networks"]:
            n["connected"] = n["ssid"] == ssid
            if n["connected"]:
                n["known"] = True
        wifi = state["network"]["wifi"]
        net = next((n for n in state["networks"] if n["ssid"] == ssid), None)
        wifi["state"] = "connected" if net else "disconnected"
        if net:
            wifi["ssid"], wifi["strength"] = ssid, net["strength"]
        else:
            wifi.pop("ssid", None)
            wifi.pop("strength", None)
        state["network"]["state"] = "connected" if net or state["network"]["ethernet"]["connected"] else "disconnected"
        snapshot = json.loads(json.dumps(state["network"]))
    publish("network.changed", snapshot)


pending = {"timer": None, "undo": None}


def display_change(kind, undo):
    """A change waits for "Keep": without it, undo after 15 s."""
    if pending["timer"]:
        pending["timer"].cancel()
    else:
        pending["undo"] = undo
    secs = 30 if kind == "screen" else 15
    pending["timer"] = threading.Timer(secs, display_revert)
    pending["timer"].start()
    with lock:
        state["display"]["pending"] = {"kind": kind, "seconds": secs}
        snapshot = json.loads(json.dumps(state["display"]))
    publish("display.pending", {"kind": kind, "seconds": secs})
    publish("display.changed", snapshot)


def display_done(event):
    if pending["timer"]:
        pending["timer"].cancel()
    pending["timer"] = None
    with lock:
        kind = state["display"].pop("pending", {}).get("kind", "")
    publish(event, {"kind": kind})
    with lock:
        snapshot = json.loads(json.dumps(state["display"]))
    publish("display.changed", snapshot)


def display_revert():
    undo, pending["undo"] = pending["undo"], None
    if undo:
        undo()
    display_done("display.reverted")


def zone_known(zone):
    try:
        zoneinfo.ZoneInfo(zone)
        return "/" in zone and ".." not in zone
    except Exception:
        return False


def zone_offsets(path):
    zones = urllib.parse.parse_qs(urllib.parse.urlparse(path).query).get("zone", [])
    now = datetime.datetime.now(datetime.timezone.utc)
    return [{"zone": z, "offset": int(now.astimezone(zoneinfo.ZoneInfo(z)).utcoffset().total_seconds())}
            for z in zones if zone_known(z)]


def pair_new():
    if not state["bluetooth"]["auto_pair"]:
        return
    publish("bluetooth.pairing", {"address": "AA:BB:CC:DD:EE:03", "name": "DualSense Wireless Controller"})
    time.sleep(1)
    with lock:
        for d in state["bluetooth"]["devices"]:
            if d["address"] == "AA:BB:CC:DD:EE:03":
                d["connected"] = True
        state["controllers"].append({"id": "aa:bb:cc:dd:ee:03", "name": "DualSense Wireless Controller",
                                     "brand": "playstation", "connection": "bluetooth", "vendor_id": "054c",
                                     "product_id": "0ce6", "has_guide": True, "battery": 45})
        state["bluetooth"]["auto_pair"] = False
    publish("bluetooth.paired", {"address": "AA:BB:CC:DD:EE:03", "name": "DualSense Wireless Controller"})
    publish("controller.added", {"id": "aa:bb:cc:dd:ee:03"})
    publish("bluetooth.auto_pair", {"enabled": False})


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def address_string(self):
        return "unix"

    def reply(self, status, body=None):
        data = b"" if body is None else json.dumps(body).encode()
        self.send_response(status)
        if body is not None:
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def error(self, status, code):
        self.reply(status, {"error": {"code": code, "message": code}})

    def body(self):
        n = int(self.headers.get("Content-Length") or 0)
        return json.loads(self.rfile.read(n) or b"{}")

    def do_GET(self):
        path = self.path
        if path == "/v1/events":
            return self.events()
        with lock:
            if path.startswith("/v1/time/zones"):
                return self.reply(200, {"zones": zone_offsets(path)})
            answers = {
                "/v1/time": state["time"],
                "/v1/xone": state["xone"],
                "/v1/status": {"session_mode": os.environ.get("FAKE_SESSION", "cage"), "owneetos_version": "preview"},
                "/v1/apps": {"apps": state["apps"], "focus": "home"},
                "/v1/steam": state["steam"],
                "/v1/update": state["update"],
                "/v1/apps/catalog": state["catalog"],
                "/v1/windows": state["windows"],
                "/v1/installs": {"installs": state["installs"]},
                "/v1/pointer": state["pointer"],
                "/v1/controllers": {"controllers": state["controllers"]},
                "/v1/network": state["network"],
                "/v1/network/wifi": {"networks": state["networks"]},
                "/v1/bluetooth": state["bluetooth"],
                "/v1/audio": state["audio"],
                "/v1/display": state["display"],
                "/v1/input/layout": {"layout": "us", "supported": ["de", "es", "fr", "it", "us"]},
            }
            answer = json.loads(json.dumps(answers.get(path))) if path in answers else None
        if answer is None:
            return self.error(404, "not_found")
        self.reply(200, answer)

    def do_POST(self):
        path, body = self.path, self.body()
        if path == "/v1/xone/install":
            with lock:
                state["xone"] = {"needed": True, "state": "installing"}
            publish("xone.changed", state["xone"])

            def installed():
                with lock:
                    state["xone"] = {"needed": False, "state": "installed"}
                publish("xone.changed", state["xone"])
            later(3, installed)
            return self.reply(202)
        if path == "/v1/network/wifi/scan":
            later(1.5, lambda: publish("wifi.scan_done"))
            return self.reply(202)
        if path == "/v1/network/wifi/connect":
            ssid = body.get("ssid", "")
            publish("network.connecting", {"ssid": ssid})
            time.sleep(1.5)
            net = next((n for n in state["networks"] if n["ssid"] == ssid), None)
            if not net:
                return self.error(404, "network.not_found")
            if net["security"] != "open" and not net["known"]:
                password = body.get("password", "")
                if password == "":
                    return self.error(422, "network.password_required")
                if not 8 <= len(password) <= 63:
                    return self.error(400, "network.invalid_password")
                if password == "wrongpassword":
                    publish("network.connect_failed", {"ssid": ssid, "error": "network.wrong_password"})
                    return self.error(422, "network.wrong_password")
            set_connected(ssid)
            return self.reply(204)
        if path == "/v1/network/wifi/disconnect":
            set_connected("")
            return self.reply(204)
        if path == "/v1/bluetooth/auto-pair":
            with lock:
                state["bluetooth"]["auto_pair"] = bool(body.get("enabled"))
            publish("bluetooth.auto_pair", {"enabled": state["bluetooth"]["auto_pair"]})
            if state["bluetooth"]["auto_pair"]:
                later(3, pair_new)
            return self.reply(204)
        if path.startswith("/v1/bluetooth/devices/") and path.endswith("/disconnect"):
            address = path.split("/")[4].upper()
            with lock:
                for d in state["bluetooth"]["devices"]:
                    if d["address"] == address:
                        d["connected"] = False
                state["controllers"] = [c for c in state["controllers"] if c["id"].upper() != address]
            publish("controller.removed", {"id": address.lower()})
            return self.reply(204)
        if path == "/v1/display/confirm":
            pending["undo"] = None
            display_done("display.confirmed")
            return self.reply(204)
        if path == "/v1/display/revert":
            display_revert()
            return self.reply(204)
        if path.startswith("/v1/power/"):
            return self.reply(204)
        if path in ("/v1/update/check", "/v1/update/apply"):
            pkgs = [{"name": "owneetd", "from": "0.11.7-1", "to": "0.11.8-1"},
                    {"name": "owneet-frontend", "from": "0.16.4-1", "to": "0.16.5-1"}]
            def result(st):
                with lock:
                    state["update"].update(st)
                publish("update.changed", dict(state["update"]))
            if path.endswith("check"):
                later(1.5, lambda: result({"state": "available", "packages": pkgs, "running": False}))
            else:
                later(0.5, lambda: result({"state": "installing", "packages": pkgs, "running": True}))
                later(3, lambda: result({"state": "done", "packages": pkgs, "running": False}))
            return self.reply(202)
        if path.startswith("/v1/apps/catalog/"):
            _, _, _, _, app_id, action = path.split("/")
            catalog = state["catalog"]
            app = next((a for a in catalog["apps"] if a["id"] == app_id), None)
            if not app:
                return self.error(404, "apps.unknown")
            if any(a.get("busy") for a in catalog["apps"]):
                return self.error(409, "apps.busy")
            installed = [a for a in catalog["apps"] if a["installed"]]

            def finish():
                with lock:
                    app.pop("busy", None)
                    app["installed"] = action == "install"
                    engine = any(a["installed"] for a in catalog["apps"])
                    for a in catalog["apps"]:
                        a["sizeMiB"] = 0 if engine else 500
                    data = json.loads(json.dumps(catalog))
                data["result"] = {"id": app_id, "action": action, "ok": True}
                publish("apps.catalog.changed", data)
            slow = (action == "install" and not installed) or (action == "remove" and installed == [app])
            if not slow:
                finish()
                return self.reply(202)
            with lock:
                app["busy"] = action
                data = json.loads(json.dumps(catalog))
            publish("apps.catalog.changed", data)
            later(3 if action == "install" else 1.5, finish)
            return self.reply(202)
        if path == "/v1/apps/launch":  # a game on a disk (4.5): its window appears after a few seconds
            app = {"id": body.get("id", "game"), "name": body.get("name", ""), "kind": "game"}
            with lock:
                state["apps"].append(app)
            publish("app.started", app)
            later(4, lambda: publish("focus.changed", {"focus": app["id"]}))
            return self.reply(201, {"id": app["id"]})
        if path == "/v1/pointer":   # 4.7: on for a running app, off by hand
            with lock:
                if body.get("on"):
                    if not any(a["id"] == body.get("app") for a in state["apps"]):
                        return self.error(404, "apps.unknown")
                    state["pointer"] = {"on": True, "app": body["app"]}
                else:
                    state["pointer"] = {"on": False, "reason": "user"}
                snapshot = dict(state["pointer"])
            publish("pointer.changed", snapshot)
            return self.reply(204)
        if path in ("/v1/input/text", "/v1/input/key", "/v1/input/pointer", "/v1/input/click"):
            return self.reply(204)
        if path.startswith("/v1/apps/") and path.endswith("/focus"):
            return self.reply(204)
        if path == "/v1/installs/inspect":
            return self.reply(200, install_inspect(body))
        if path == "/v1/installs":
            info = install_inspect(body)
            with lock:
                first = not state["installs"]
                state["installs"].append({"dest": body["dest"], "title": body.get("title", ""), "kind": body["kind"],
                                          "installer": body["installer"], "state": "installing" if first else "queued",
                                          "done": 0, "total": info["need"] if body["kind"] == "sh" else 0})
                snapshot = installs_copy()
            publish("installs.changed", snapshot)
            if first:
                later(1, install_tick)
            return self.reply(202)
        if path == "/v1/installs/stop":
            with lock:
                gone = [i for i in state["installs"] if i["dest"] == body.get("dest")]
                state["installs"] = [i for i in state["installs"] if i["dest"] != body.get("dest")]
                if gone and gone[0]["state"] == "installing":
                    nxt = next((i for i in state["installs"] if i["state"] == "queued"), None)
                    if nxt:
                        nxt["state"] = "installing"
                snapshot = installs_copy()
            if not gone:
                return self.error(404, "installs.not_found")
            publish("installs.changed", snapshot)
            publish("install.failed", {"dest": gone[0]["dest"], "title": gone[0]["title"], "reason": "cancelled"})
            return self.reply(204)
        if path == "/v1/installs/uninstall":
            dest = body.get("dest", "")
            if not os.path.exists(os.path.join(dest, ".owneet-installed")):
                return self.error(404, "installs.not_found")
            shutil.rmtree(dest)
            return self.reply(204)
        if path == "/v1/windows/needed":
            with lock:
                w = state["windows"]
                first = not w["needed"]
                w["needed"] = True
                if first:
                    w["state"] = "downloading"
            if first:
                later(1, windows_tick)
            return self.reply(204)
        if path == "/v1/steam":
            req = body
            if req.get("ask"):     # 4.2c: the next start asks the user first (Steam shown at once)
                return self.reply(202)
            with lock:
                steam = state["steam"]
                if not steam["allowed"]:
                    return self.error(409, "steam.needs_install")
                # 4.2c: Steam's confirmation is accepted at once; downloads move on by themselves
                if req.get("install"):
                    later(1.5, lambda: steam_install(req["install"]))
                elif req.get("uninstall"):
                    later(1.5, lambda: steam_uninstall(req["uninstall"]))
                starting = not steam["running"]
                steam["running"] = True
                if starting:
                    state["apps"].append({"id": "steam", "name": "Steam", "kind": "app"})
            if starting:   # Steam's window appears after a while (minutes the first time)
                publish("app.started", {"id": "steam", "name": "Steam", "kind": "app"})
                later(6, lambda: (state["steam"].update(prepared=True), publish("app.window", {"id": "steam"})))
            return self.reply(202)
        self.error(404, "not_found")

    def do_PUT(self):
        path, body = self.path, self.body()
        if path == "/v1/time/zone":
            zone = body.get("zone", "")
            if not zone_known(zone):
                return self.error(400, "time.unknown_zone")
            with lock:
                state["time"]["zone"] = zone
            publish("time.changed", state["time"])
            return self.reply(200, state["time"])
        if path == "/v1/network/wifi/enabled":
            with lock:
                state["network"]["wifi"]["enabled"] = bool(body.get("enabled"))
            set_connected("Casa Rossi" if body.get("enabled") else "")
            return self.reply(204)
        if path == "/v1/display/mode":
            mode = body.get("mode", "")
            if mode != "auto" and mode not in state["display"]["modes"]:
                return self.error(400, "display.unknown_mode")
            before = state["display"]["mode"]
            state["display"]["mode"] = mode
            display_change("mode", lambda: state["display"].update(mode=before))
            return self.reply(204)
        if path == "/v1/display/screen":
            target = body.get("id", "")
            if target not in [x["id"] for x in state["display"]["screens"]]:
                return self.error(404, "display.unknown_screen")
            before = next(x["id"] for x in state["display"]["screens"] if x["active"])

            def use(screen_id):
                for x in state["display"]["screens"]:
                    x["active"] = x["id"] == screen_id
            use(target)
            display_change("screen", lambda: use(before))
            return self.reply(204)
        if path == "/v1/display/brightness":
            state["display"]["brightness"] = int(body.get("value", 0))
            return self.reply(204)
        if path in ("/v1/audio/mute", "/v1/audio/volume"):
            with lock:
                state["audio"].update(body)
        if path.startswith("/v1/audio/") or path == "/v1/input/layout":
            publish("input.layout_changed" if "layout" in path else "audio.changed", body)
            return self.reply(204)
        self.error(404, "not_found")

    def do_DELETE(self):
        path = self.path
        if path.startswith("/v1/network/wifi/"):
            ssid = urllib.parse.unquote(path[len("/v1/network/wifi/"):])
            with lock:
                for n in state["networks"]:
                    if n["ssid"] == ssid:
                        n["known"] = False
            if state["network"]["wifi"].get("ssid") == ssid:
                set_connected("")
            publish("network.forgotten", {"ssid": ssid})
            return self.reply(204)
        if path.startswith("/v1/bluetooth/devices/"):
            address = path.split("/")[4].upper()
            with lock:
                state["bluetooth"]["devices"] = [d for d in state["bluetooth"]["devices"] if d["address"] != address]
                state["controllers"] = [c for c in state["controllers"] if c["id"].upper() != address]
            publish("bluetooth.forgotten", {"address": address})
            publish("controller.removed", {"id": address.lower()})
            return self.reply(204)
        self.error(404, "not_found")

    def events(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        q = queue.Queue()
        with lock:
            listeners.append(q)
        try:
            while True:
                kind, data = q.get()
                block = json.dumps({"type": kind, "data": data})
                self.wfile.write(f"event: {kind}\ndata: {block}\n\n".encode())
                self.wfile.flush()
        except OSError:
            pass
        finally:
            with lock:
                listeners.remove(q)


class Server(socketserver.ThreadingMixIn, socketserver.UnixStreamServer):
    daemon_threads = True


def read_events(path):
    while True:
        with open(path) as fifo:
            for line in fifo:
                kind, _, data = line.strip().partition(" ")
                if not kind:
                    continue
                payload = json.loads(data) if data else None
                with lock:            # games and apps announced by events are "running"
                    if kind == "fake.steam":
                        state["steam"].update(payload)
                        kind, payload = "steam.changed", json.loads(json.dumps(state["steam"]))
                    if kind == "app.started":
                        state["apps"].append(payload)
                    elif kind == "app.exited":
                        state["apps"] = [a for a in state["apps"] if a["id"] != payload["id"]]
                publish(kind, payload)


if __name__ == "__main__":
    sock = sys.argv[1]
    if os.environ.get("FAKE_FIRSTSTART"):   # a new console: no network joined yet
        state["network"]["state"] = "disconnected"
        state["network"]["wifi"].update({"state": "disconnected", "ssid": "", "strength": 0})
        for n in state["networks"]:
            n["connected"] = False
            n["known"] = False
    if os.environ.get("FAKE_STEAM"):   # the invented Steam folder of fake-steam.py
        state["steam"].update({"prepared": True, "account": True, "nvidia": True, "games": ["2001", "2002", "2010"],
                               "away": {"2003": "Games SSD"}})
    if len(sys.argv) > 2:
        threading.Thread(target=read_events, args=(sys.argv[2],), daemon=True).start()
    if os.path.exists(sock):
        os.remove(sock)
    Server(sock, Handler).serve_forever()
