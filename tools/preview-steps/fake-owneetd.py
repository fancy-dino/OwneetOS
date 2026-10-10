#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
#
# A pretend owneetd for tools/frontend-preview (steps files starting with "@owneetd"): the same
# HTTP + JSON API on a Unix socket, with invented networks, controllers and Bluetooth devices,
# so that the Settings panels and windows can be seen without Wi-Fi or Bluetooth hardware.
# It is not the daemon: only what the interface asks, and simple made-up behaviour:
#   - Wi-Fi connect: password "wrongpassword" → wrong_password, other valid ones → connected (1.5 s);
#   - auto-pair on: after 3 s a "DualSense Wireless Controller" pairs;
#   - scan: wifi.scan_done after 1.5 s.
#
#   fake-owneetd.py SOCKET

import json
import os
import queue
import socketserver
import sys
import threading
import time
import urllib.parse
from http.server import BaseHTTPRequestHandler

lock = threading.Lock()
listeners = []

state = {
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
    "audio": {"available": True, "output": "hdmi", "volume": 60, "muted": False,
              "outputs": [{"id": "hdmi", "name": "TV (HDMI)"}, {"id": "speakers", "name": "Speakers"}]},
}


def publish(kind, data=None):
    with lock:
        for q in listeners:
            q.put((kind, data))


def later(seconds, fn):
    threading.Timer(seconds, fn).start()


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
            answers = {
                "/v1/status": {"session_mode": "cage", "owneetos_version": "preview"},
                "/v1/apps": {"apps": [], "focus": ""},
                "/v1/controllers": {"controllers": state["controllers"]},
                "/v1/network": state["network"],
                "/v1/network/wifi": {"networks": state["networks"]},
                "/v1/bluetooth": state["bluetooth"],
                "/v1/audio": state["audio"],
                "/v1/input/layout": {"layout": "us", "supported": ["it", "us"]},
            }
            answer = json.loads(json.dumps(answers.get(path))) if path in answers else None
        if answer is None:
            return self.error(404, "not_found")
        self.reply(200, answer)

    def do_POST(self):
        path, body = self.path, self.body()
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
        if path.startswith("/v1/power/"):
            return self.reply(204)
        self.error(404, "not_found")

    def do_PUT(self):
        path, body = self.path, self.body()
        if path == "/v1/network/wifi/enabled":
            with lock:
                state["network"]["wifi"]["enabled"] = bool(body.get("enabled"))
            set_connected("Casa Rossi" if body.get("enabled") else "")
            return self.reply(204)
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


if __name__ == "__main__":
    sock = sys.argv[1]
    if os.path.exists(sock):
        os.remove(sock)
    Server(sock, Handler).serve_forever()
