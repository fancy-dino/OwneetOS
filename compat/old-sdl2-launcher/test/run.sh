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
$cc -O2 -Wall -Wextra -Werror -o "$out/launcher" "$here/launcher.c" -L"$out" -lfake -Wl,-rpath,'$ORIGIN'
lib="$out/libowneet-old-sdl2-launcher.so"
run() { LD_PRELOAD="$lib" "$out/launcher" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

ms="$(run wait | head -1)"
[[ "$ms" -ge 10 ]] || fail "a short wait with no event lasted $ms ms, not 10"
[[ "$("$out/launcher" wait | head -1)" -lt 10 ]] || fail "without the library the wait should be short"
[[ "$(FAKE_EVENT=0x100 run poll)" == "" ]] || fail "SDL_QUIT did not end the program"
[[ "$(FAKE_EVENT=0x300 run poll)" == "went on" ]] || fail "another event ended the program"
[[ "$(run spawn)" == "" ]] || fail "starting the game did not end the program"
[[ "$(run env)" == "[]" ]] || fail "the library stayed in LD_PRELOAD"
[[ "$(LD_PRELOAD="/other.so:$lib" "$out/launcher" env 2>/dev/null)" == "[/other.so]" ]] || fail "other preloads were not kept"
echo "old-sdl2-launcher: all checks passed"
