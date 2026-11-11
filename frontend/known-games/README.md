# frontend/known-games/

`known-games.json`: the games OwneetOS knows, installed as
`/usr/share/owneet-frontend/known-games.json` ([[games-on-disk]], owner 2026-11-11). For a game
folder on a disk, it names the file to start when GOG's `start.sh` does not start the game itself.
A file chosen from Options and `game.txt`'s `start` win over it.

```json
{ "gog": { "<GOG id>": { "title": "<the game, for whoever reads this file>", "start": "<file, relative to the game's folder>" } } }
```

The GOG id is the 5th line of GOG's `gameinfo`, at the top of an installed GOG Linux game.

| Game | File | Why |
|---|---|---|
| The Witcher 2 (GOG, Linux) | `game/witcher2` | `start.sh` opens `desktop-launcher`, which needs 32-bit GTK 2; `tenfoot-launcher` crashes on its own language files (open-items 3u). |
