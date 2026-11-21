# frontend/known-games/

`known-games.json`: the games OwneetOS knows, installed as
`/usr/share/owneet-frontend/known-games.json` ([[games-on-disk]], owner 2026-11-11). For a game
folder on a disk:

- `start`: the file to start when GOG's `start.sh` does not work. A file chosen from Options and
  `game.txt`'s `start` win over it.
- `fix-key-files`: the game's GLib key files (a folder and a `*` pattern, relative to the game's
  folder) whose values have a lone backslash that today's GLib refuses; the interface writes it
  twice (`\` → `\\`), on any disk the console can write and only in files that need it.
- `launchers` (owner, 2026-11-12): for a file of the game, `compat`, the OwneetOS library it is
  started with (`compat/`, installed as `/usr/lib32/owneet/libowneet-<name>.so`, `LD_PRELOAD`), and
  `hands-over`, a launcher that starts the game and ends: the app then lasts while any of its
  programs runs (only for the games listed here); `game-compat`, the OwneetOS library the game it
  starts gets (handed over by the launcher's library: `OWNEET_GAME_PRELOAD`).

```json
{ "gog": { "<GOG id>": { "title": "<the game, for whoever reads this file>", "start": "<file>",
  "fix-key-files": ["<folder/*.ini>"],
  "launchers": { "<file>": { "compat": "<library>", "game-compat": "<library>", "hands-over": true } } } } }
```

The GOG id is the 5th line of GOG's `gameinfo`, at the top of an installed GOG Linux game.

| Game | Start | Fix | Launcher | Why |
|---|---|---|---|---|
| The Witcher 2 (GOG, Linux) | `game/tenfoot-launcher` (the launcher for the TV and the controller, where the language is chosen) | `game/bin/config/lang/*.ini` | `old-sdl2-launcher`, hands over; the game gets `no-crash-reporter` | `start.sh` opens `desktop-launcher`, which needs 32-bit GTK 2; `tenfoot-launcher` aborts on `IK_Backslash=\ (…)` in its language files, stays black under SDL3 (its loop needs SDL2's 10 ms wait), crashes in its clean-up after starting the game and ignores the request to close; the game crashes when quit from its menu and its 2015 crash reporter shows a desktop window (open-items 3u). |
