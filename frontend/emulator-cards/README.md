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
- `settings`: files written only when they are not there, when they are as the console wrote them, or when they are still
  the emulator's own first settings (`replaceWhen`, a regular expression: e.g. its keyboard); the user's changes are never
  overwritten. `section`: only that section of an ini file (added when it is not there; the rest is kept). `~/` is the home
  folder. The first controller, as SDL sees it on the console: `{pad.name}`; the numbers of its own buttons in the standard
  layout `{pad.a}` `{pad.b}` `{pad.x}` `{pad.y}` `{pad.back}` `{pad.start}` `{pad.lb}` `{pad.rb}` `{pad.ls}` `{pad.rs}`
  `{pad.up}` `{pad.down}` `{pad.left}` `{pad.right}` (-1 when a direction is a hat's: `{pad.hat}`), of its axes `{pad.lx}`
  `{pad.ly}` `{pad.rx}` `{pad.ry}`, and `{pad.hidapi}` (1 when SDL drives it itself). With no controller they wait.
- `env`: the emulator's environment (`{pad.…}` too): e.g. `"SDL_JOYSTICK_HIDAPI": "{pad.hidapi}"`, so that an emulator that
  reads the controller's own buttons sees them numbered as the console does.
