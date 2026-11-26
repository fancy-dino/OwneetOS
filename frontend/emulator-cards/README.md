# Emulator cards

What the console knows of an emulator it recognises ([[step-4.9-the-users-own-emulators]], [[users-own-emulators]]):
how it starts a game, which of a game's files it wants, the system file it needs and how that is installed, and the
settings it gets when it has none of its own (its controller). The user still brings the emulator, its system files and
the games; the console ships, downloads and links none.

**The cards are kept out of git until the legal review** (owner, 2026-11-26: `*.json` here is ignored). They are packaged
with the interface when they are here (`/usr/share/owneet-frontend/emulator-cards/`); an emulator without a card works as
before. `OWNEET_EMULATOR_CARDS` (folders, separated by `:`) replaces the place, for tests and previews.

## A card: one JSON file

```json
{
  "match": ["boxstation*"],
  "start": ["--fullscreen", "{game}"],
  "prefer": "folder",
  "systemFile": {
    "name": "SYSTEM.BIN",
    "installed": ["~/.config/boxstation/system/ready.dat"],
    "install": ["--install", "{file}"]
  },
  "settings": [
    { "path": "~/.config/boxstation/pad.ini", "text": "[Pad1]\nDevice = {pad.name}\nA = South\n" }
  ]
}
```

- `match`: the file name of the emulator's program, or of its folder, case left aside (`*` and `?`).
- `start`: how it starts a game when the user has not chosen in "How it starts"; `{game}` is the game's file or folder.
- `prefer`: which of a game's files when it has several and the user has not chosen: `"folder"`, or names (`["*.iso"]`).
- `systemFile`: the file the user copies beside the emulator (`name`, case left aside); `installed`, any of these paths there
  means it is installed; `install`, the emulator's options that install it (`{file}`), run by themselves before the first game.
- `settings`: files written only when they are not there, or when they are as the console wrote them (the user's changes are
  never overwritten). `{pad.name}` is the name of the first controller, as SDL gives it; with no controller they wait.
  `~/` is the home folder.
