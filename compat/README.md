# compat/

OwneetOS's own compatibility libraries for old Linux games, packaged as `lib32-owneet-compat`.
Each one is loaded (`LD_PRELOAD`) **only** for the files the list of known games names
(`frontend/known-games/`, [[games-on-disk]]); no game's files are changed.

| Library | Installed as | For |
|---|---|---|
| `old-sdl2-launcher/` | `/usr/lib32/owneet/libowneet-old-sdl2-launcher.so` | Old launchers made for SDL2 (The Witcher 2's TV launcher): SDL2's 10 ms event wait, which their screen needs under SDL3; memory they delete kept as it was (they read it afterwards); the controller turned into their keys (D-pad and left stick = arrows, A = Enter, B = Esc), never seen by their own broken controller handling; they end once they have started the game, and when the console closes them. |
| `no-crash-reporter/` | `/usr/lib32/owneet/libowneet-no-crash-reporter.so` | Games whose own crash reporter cannot start and shows a desktop window (The Witcher 2): it is not started (nothing is sent); handed to the game by its launcher (`OWNEET_GAME_PRELOAD`). |

Tests: `old-sdl2-launcher/test/run.sh BUILD_DIR`, `no-crash-reporter/test/run.sh BUILD_DIR` (run by the package's `check()`).
