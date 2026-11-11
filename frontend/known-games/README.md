# frontend/known-games/

`known-games.json`: the games OwneetOS knows, installed as
`/usr/share/owneet-frontend/known-games.json` ([[games-on-disk]], owner 2026-11-11). For a game
folder on a disk:

- `start`: the file to start when GOG's `start.sh` does not work. A file chosen from Options and
  `game.txt`'s `start` win over it.
- `fix-key-files`: the game's GLib key files (a folder and a `*` pattern, relative to the game's
  folder) whose values have a lone backslash that today's GLib refuses; the interface writes it
  twice (`\` → `\\`), only on the console's disk and only in files that need it.

```json
{ "gog": { "<GOG id>": { "title": "<the game, for whoever reads this file>", "start": "<file>", "fix-key-files": ["<folder/*.ini>"] } } }
```

The GOG id is the 5th line of GOG's `gameinfo`, at the top of an installed GOG Linux game.

| Game | Start | Fix | Why |
|---|---|---|---|
| The Witcher 2 (GOG, Linux) | `game/tenfoot-launcher` (the launcher for the TV and the controller, where the language is chosen) | `game/bin/config/lang/*.ini` | `start.sh` opens `desktop-launcher`, which needs 32-bit GTK 2; `tenfoot-launcher` aborts on `IK_Backslash=\ (…)` in its language files (open-items 3u). |
