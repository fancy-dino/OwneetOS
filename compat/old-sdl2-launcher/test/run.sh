#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Builds the library and the test program (32-bit), then checks the library's three corrections.
# Usage: compat/old-sdl2-launcher/test/run.sh BUILD_DIR
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
out="${1:?build folder}"
mkdir -p "$out"
cc="${CC:-gcc -m32}"
$cc -shared -fPIC -O2 -Wall -Wextra -Werror -o "$out/libowneet-old-sdl2-launcher.so" "$here/../old-sdl2-launcher.c" -ldl
$cc -shared -fPIC -O2 -Wall -Wextra -Werror -o "$out/libfake.so" "$here/fake.c"
# shellcheck disable=SC2016  # $ORIGIN is for the linker, not the shell
$cc -O2 -Wall -Wextra -Werror -o "$out/launcher" "$here/launcher.c" -L"$out" -lfake -lstdc++ -Wl,-rpath,'$ORIGIN'
lib="$out/libowneet-old-sdl2-launcher.so"
run() { LD_PRELOAD="$lib" "$out/launcher" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

ms="$(run wait | head -1)"
[[ "$ms" -ge 10 ]] || fail "a short wait with no event lasted $ms ms, not 10"
[[ "$("$out/launcher" wait | head -1)" -lt 10 ]] || fail "without the library the wait should be short"
[[ "$(FAKE_EVENTS=0x100 run poll)" == "" ]] || fail "SDL_QUIT did not end the program"
[[ "$(FAKE_EVENTS=0x300 run poll)" == "down:0 opened=0" ]] || fail "a key event was changed"
# a controller added (opened by the library), A, B and the D-pad: Enter, Esc, arrows; joystick events hidden
got="$(FAKE_EVENTS=0x605,0x653,0x651:0,0x652:0,0x651:1,0x651:12,0x652:12,0x651:14,0x651:3 run poll)"
[[ "$got" == "down:13 up:13 down:27 down:81 up:81 down:79 opened=1" ]] || fail "buttons: $got"
# the left stick: down past half, then back; left, then straight right (left released, right pressed)
got="$(FAKE_EVENTS=0x650:1:20000,0x650:1:12000,0x650:1:0,0x650:0:-20000,0x650:0:20000,0x650:0:100 run poll)"
[[ "$got" == "down:81 up:81 down:80 up:80 down:79 up:79 opened=0" ]] || fail "stick: $got"
[[ "$(run open)" == "joystick=none controller=none" ]] || fail "the launcher could open a controller"
[[ "$(run delete)" == "kept" ]] || fail "deleted memory was not kept"
[[ "$("$out/launcher" delete)" == "changed" ]] || echo "note: without the library glibc left the block as it was"
[[ "$(run spawn)" == "" ]] || fail "starting the game did not end the program"
[[ "$(run env)" == "[]" ]] || fail "the library stayed in LD_PRELOAD"
[[ "$(LD_PRELOAD="/other.so:$lib" "$out/launcher" env 2>/dev/null)" == "[/other.so]" ]] || fail "other preloads were not kept"
[[ "$(OWNEET_GAME_PRELOAD=/game.so run env)" == "[/game.so]" ]] || fail "the game's library did not take its place"
echo "old-sdl2-launcher: all checks passed"
