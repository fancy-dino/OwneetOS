# SPDX-License-Identifier: GPL-3.0-or-later
"""The player's preview (tools/player-preview), inside its container: invented media made with
FFmpeg, the session the console sends (texts from the message files, the palette from
Theme.qml, the language codes from Media.cpp), mpv started as owneetd starts it, and the steps:
buttons through mpv's control socket, screenshots, what the player tells owneetd."""

import hashlib
import json
import os
import re
import socket
import subprocess
import sys
import threading
import time

MEDIA = "/media"
MEDIA_VERSION = "3"           # made again when this changes
HOME = "/tmp/home"
RUN = "/tmp/run/owneet"
WATCH_LATER = HOME + "/.local/state/owneet/watch_later"
SOCK = RUN + "/player.sock"
SESSION = RUN + "/player.json"
OUT = "/out"

PAD = {"a": "GAMEPAD_ACTION_DOWN", "b": "GAMEPAD_ACTION_RIGHT", "x": "GAMEPAD_ACTION_LEFT",
       "y": "GAMEPAD_ACTION_UP", "lb": "GAMEPAD_LEFT_SHOULDER", "rb": "GAMEPAD_RIGHT_SHOULDER",
       "lt": "GAMEPAD_LEFT_TRIGGER", "rt": "GAMEPAD_RIGHT_TRIGGER", "menu": "GAMEPAD_START",
       "view": "GAMEPAD_BACK", "guide": "GAMEPAD_MENU", "up": "GAMEPAD_DPAD_UP",
       "down": "GAMEPAD_DPAD_DOWN", "left": "GAMEPAD_DPAD_LEFT", "right": "GAMEPAD_DPAD_RIGHT",
       "l3": "GAMEPAD_LEFT_STICK", "r3": "GAMEPAD_RIGHT_STICK"}

# The keys the console sends (theme.qml, playMedia): the player's own texts
TEXTS = ["player.play", "player.pause", "player.seek", "player.prevnext", "prompt.back",
         "prompt.home", "player.hold", "player.state.audio", "player.state.subtitles",
         "player.off", "media.autoplay", "media.on", "media.off", "player.track",
         "player.forward", "player.backward", "player.start", "player.last.video",
         "player.last.song", "player.audio", "player.subtitles", "player.resumed",
         "player.failed", "media.unknown_artist", "player.next", "player.last_song"]


def ff(*args):
    subprocess.run(["ffmpeg", "-y", "-hide_banner", "-loglevel", "error", *args], check=True)


def make_media():
    if os.path.exists(f"{MEDIA}/.done-{MEDIA_VERSION}"):
        return
    subprocess.run(["rm", "-rf", MEDIA])
    print("== making the invented media (once)", flush=True)
    season = MEDIA + "/Videos/Harbour Tales/Season 1"
    os.makedirs(season, exist_ok=True)
    srt = "/tmp/sub-{}.srt"
    # a line every 4 s, as in a film (mpv shows an embedded track from its next line on)
    lines = {"en": ["The boat leaves at dawn.", "Did you see the light on the water?", "We have until the tide turns.",
                    "Keep the lantern lit."],
             "it": ["La barca parte all’alba.", "Hai visto la luce sull’acqua?", "Abbiamo tempo fino alla marea.",
                    "Tieni accesa la lanterna."]}
    for lang, words in lines.items():
        with open(srt.format(lang), "w") as f:
            for n in range(22):
                f.write(f"{n + 1}\n00:{4 * n // 60:02d}:{4 * n % 60:02d},500 --> 00:{(4 * n + 3) // 60:02d}:{(4 * n + 3) % 60:02d},500\n"
                        f"{words[n % 4]}\n\n")
    scene = ["-f", "lavfi", "-i", "gradients=s=960x540:r=25:c0=0x1d3557:c1=0x457b9d:c2=0xe9c46a:speed=0.02"]
    common = ["-c:v", "libx264", "-preset", "ultrafast", "-g", "200", "-pix_fmt", "yuv420p", "-c:a", "aac"]
    ff(*scene, "-f", "lavfi", "-i", "sine=f=440", "-f", "lavfi", "-i", "sine=f=660",
       "-i", srt.format("en"), "-i", srt.format("it"), "-t", "90",
       "-map", "0:v", "-map", "1:a", "-map", "2:a", "-map", "3:s", "-map", "4:s", *common, "-c:s", "srt",
       "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=ita",
       "-metadata:s:s:0", "language=eng", "-metadata:s:s:1", "language=ita",
       season + "/Episode 1 - The Lantern.mkv")
    ff(*scene, "-f", "lavfi", "-i", "sine=f=520", "-t", "60", "-map", "0:v", "-map", "1:a", *common,
       "-metadata:s:a:0", "language=eng", season + "/Episode 2 - Low Water.mkv")
    with open(season + "/Episode 2 - Low Water.en.srt", "w") as f:
        f.write("1\n00:00:01,000 --> 00:00:55,000\nA subtitle file beside the video.\n")
    ff(*scene, "-f", "lavfi", "-i", "sine=f=330", "-t", "20", "-map", "0:v", "-map", "1:a", *common,
       season + "/Episode 3 - Night Ferry.mkv")

    album = MEDIA + "/Music/Coral Avenue/Low Tide"
    other = MEDIA + "/Music/Signal Fires/Northbound"
    loose = MEDIA + "/Music/Mixtape"
    for d in (album, other, loose):
        os.makedirs(d, exist_ok=True)
    ff("-f", "lavfi", "-i", "gradients=s=600x600:c0=0x2a9d8f:c1=0x264653:x0=0:y0=0:x1=600:y1=600",
       "-frames:v", "1", album + "/cover.jpg")
    ff("-f", "lavfi", "-i", "gradients=s=500x500:c0=0xe76f51:c1=0x6d2e46:x0=0:y0=0:x1=500:y1=500",
       "-frames:v", "1", "/tmp/embedded.jpg")
    tags = lambda title, artist, albm, n: ["-metadata", f"title={title}", "-metadata", f"artist={artist}",
                                         "-metadata", f"album_artist={artist}", "-metadata", f"album={albm}",
                                         "-metadata", f"track={n}"]
    ff("-f", "lavfi", "-i", "sine=f=440:d=50", "-c:a", "flac",
       *tags("Shoreline", "Coral Avenue", "Low Tide", 1), album + "/01 - Shoreline.flac")
    ff("-f", "lavfi", "-i", "sine=f=494:d=40", "-c:a", "libmp3lame", "-q:a", "5",
       *tags("Undertow", "Coral Avenue", "Low Tide", 2), album + "/02 - Undertow.mp3")
    ff("-f", "lavfi", "-i", "sine=f=587:d=40", "-i", "/tmp/embedded.jpg", "-map", "0:a", "-map", "1:v",
       "-c:a", "libmp3lame", "-q:a", "5", "-c:v", "mjpeg", "-disposition:v", "attached_pic", "-id3v2_version", "3",
       *tags("Ember", "Signal Fires", "Northbound", 1), other + "/01 - Ember.mp3")
    ff("-f", "lavfi", "-i", "sine=f=659:d=30", "-c:a", "libmp3lame", "-q:a", "5", loose + "/Rain on the roof.mp3")
    open(f"{MEDIA}/.done-{MEDIA_VERSION}", "w").close()


VIDEOS = [("Harbour Tales/Season 1/Episode 1 - The Lantern.mkv", "Episode 1 - The Lantern"),
          ("Harbour Tales/Season 1/Episode 2 - Low Water.mkv", "Episode 2 - Low Water"),
          ("Harbour Tales/Season 1/Episode 3 - Night Ferry.mkv", "Episode 3 - Night Ferry")]
SONGS = [("Coral Avenue/Low Tide/01 - Shoreline.flac", "Shoreline", "Coral Avenue", "Low Tide"),
         ("Coral Avenue/Low Tide/02 - Undertow.mp3", "Undertow", "Coral Avenue", "Low Tide"),
         ("Signal Fires/Northbound/01 - Ember.mp3", "Ember", "Signal Fires", "Northbound"),
         ("Mixtape/Rain on the roof.mp3", "Rain on the roof", "", "Mixtape")]


def hue(seed):
    """GameArt's: FNV-1a over the title's characters, spread over the colour wheel."""
    h = 2166136261
    for ch in seed.encode("utf-16-le").decode("utf-16-le"):
        h = ((h ^ ord(ch)) * 16777619) & 0xFFFFFFFF
    return h % 360


def session(kind, opts):
    msgs = json.load(open(f"/preview/i18n/{opts['lang']}.json"))
    english = json.load(open("/preview/i18n/en.json"))
    tr = lambda k: msgs.get(k, english.get(k, k))
    texts = {k: tr(k) for k in TEXTS}
    texts["label.playing"] = tr("home.playing").upper()
    theme = open("/preview/Theme.qml").read()
    m = re.search(r'key: "%s".*?c: \[([^\]]+)\]' % re.escape(opts["palette"]), theme)
    c = re.findall(r'"(#[0-9A-Fa-f]{6})"', m.group(1))
    look = dict(zip(["bg", "surface", "raised", "line", "fg", "muted", "accent", "onAccent"], c))
    look.update(displayFont="Bricolage Grotesque", textFont="Lexend", textScale=opts["textscale"],
                glyphs=opts["glyphs"])
    codes = dict(re.findall(r'\{ "([a-z]{3})", "([a-z]{2})" \}', open("/preview/Media.cpp").read()))
    languages = {k[5:]: v for k, v in msgs.items() if k.startswith("lang.")}
    if kind == "videos":
        items = [{"path": f"{MEDIA}/Videos/{p}", "title": t, "folder": "Harbour Tales › Season 1",
                  "hue": hue("Harbour Tales")} for p, t in VIDEOS]
    else:
        items = [{"path": f"{MEDIA}/Music/{p}", "title": t, "artist": a, "album": al, "hue": hue(al)}
                 for p, t, a, al in SONGS]
    return {"kind": kind, "mode": opts["session"], "autoplay": opts["autoplay"], "items": items,
            "texts": texts, "languages": languages, "codes": codes, "look": look}


def watch_later(path):
    return os.path.join(WATCH_LATER, hashlib.md5(path.encode()).hexdigest().upper())


class Player:
    def __init__(self, kind, opts, start, from_start):
        self.s = session(kind, opts)
        os.makedirs(RUN, exist_ok=True)
        os.makedirs(WATCH_LATER, exist_ok=True)
        if from_start:
            try:
                os.remove(watch_later(self.s["items"][start]["path"]))
            except FileNotFoundError:
                pass
        json.dump(self.s, open(SESSION, "w"), ensure_ascii=False)
        # as owneetd builds it (daemon/internal/player/player.go, args)
        args = ["mpv", "--config-dir=/usr/share/owneet/player", f"--input-ipc-server={SOCK}",
                f"--script-opts=owneet-session={SESSION}", f"--watch-later-dir={WATCH_LATER}"]
        if kind == "music":
            args.append("--profile=owneet-music")
        # The preview only: X11 output, the last of the list owneetd gives under gamescope (the
        # container's software OpenGL is captured half-drawn); no sound device, and without
        # --ao=null mpv drops the audio track (the console plays through PipeWire)
        args += ["--vo=x11", f"--playlist-start={start}", "--ao=null", "--"]
        args += [i["path"] for i in self.s["items"]]
        self.log = open(OUT + "/log.txt", "a")
        self.proc = subprocess.Popen(args, stdout=self.log, stderr=subprocess.STDOUT,
                                     env=dict(os.environ, HOME=HOME, XDG_RUNTIME_DIR="/tmp/run"))
        self.state, self.ids = None, 0
        for _ in range(100):
            if os.path.exists(SOCK):
                break
            time.sleep(0.1)
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.connect(SOCK)
        threading.Thread(target=self.read, daemon=True).start()

    def read(self):
        buf = b""
        while True:
            try:
                data = self.sock.recv(65536)
            except OSError:
                return
            if not data:
                return
            buf += data
            while b"\n" in buf:
                line, buf = buf.split(b"\n", 1)
                ev = json.loads(line)
                if ev.get("event") == "client-message" and ev.get("args", [""])[0] == "owneet-state":
                    self.state = json.loads(ev["args"][1])

    def command(self, *args):
        self.ids += 1
        try:
            self.sock.sendall((json.dumps({"command": list(args), "request_id": self.ids}) + "\n").encode())
        except OSError:
            print("   (the player has quit)")


def main(steps):
    opts = {"lang": "en", "palette": "dusk", "glyphs": "xbox", "textscale": 1.0, "session": "gamescope",
            "autoplay": True}
    lines = [l.strip() for l in open(steps) if l.strip()]
    kind = "music" if lines and lines[0] == "@music" else "videos"
    make_media()
    subprocess.run(["rm", "-rf", HOME, "/tmp/run"])
    os.makedirs(WATCH_LATER, exist_ok=True)
    player = None
    for line in lines:
        w = line.split()
        print("--", line, flush=True)
        if w[0].startswith(("#", "@")):
            continue
        if w[0] in ("lang", "palette", "glyphs", "session"):
            opts[w[0]] = w[1]
        elif w[0] == "textscale":
            opts["textscale"] = float(w[1])
        elif w[0] == "autoplay":
            opts["autoplay"] = w[1] == "on"
        elif w[0] == "saved":
            path = session(kind, opts)["items"][int(w[1])]["path"]
            with open(watch_later(path), "w") as f:
                f.write(f"# {path}\nstart={float(w[2]):.6f}\n")
        elif w[0] == "start":
            player = Player(kind, opts, int(w[1]), len(w) > 2 and w[2] == "fromstart")
            time.sleep(2.5)
        elif w[0] == "pad":
            for b in w[1:]:
                player.command("keypress", PAD[b])
                time.sleep(0.4)
        elif w[0] == "key":
            for k in w[1:]:
                player.command("keypress", k)
                time.sleep(0.4)
        elif w[0] == "hold":
            name = PAD.get(w[1], w[1])
            player.command("keydown", name)
            time.sleep(int(w[2]) / 1000)
            player.command("keyup", name)
            time.sleep(0.3)
        elif w[0] == "focus":
            player.command("script-message", "owneet-focus", w[1])
            time.sleep(0.3)
        elif w[0] == "seek":
            player.command("seek", w[1], "absolute+exact")
            time.sleep(0.6)
        elif w[0] == "shot":
            time.sleep(0.5)
            subprocess.run(["import", "-window", "root", f"{OUT}/{w[1]}.png"], check=False)
        elif w[0] == "wait":
            time.sleep(float(w[1]))
        elif w[0] == "state":
            print("   state:", json.dumps(player.state, ensure_ascii=False))
        elif w[0] == "files":
            names = sorted(os.listdir(WATCH_LATER))
            for n in names:
                text = open(os.path.join(WATCH_LATER, n)).read().strip().replace("\n", " | ")
                if "redirect entry" not in text:
                    print("   saved:", text)
            if not any("redirect entry" not in open(os.path.join(WATCH_LATER, n)).read() for n in names):
                print("   saved: (none)")
        elif w[0] == "exit":
            try:
                print("   mpv quit:", player.proc.wait(timeout=15))
            except subprocess.TimeoutExpired:
                print("   mpv is still running")
        else:
            print("   unknown step")
    if player and player.proc.poll() is None:
        player.command("quit")
        player.proc.wait(timeout=10)


if __name__ == "__main__":
    main(sys.argv[1])
