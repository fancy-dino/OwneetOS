#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Builds the library and a test program (32-bit), then checks that only the crash reporter is
# stopped. Usage: compat/no-crash-reporter/test/run.sh BUILD_DIR
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
out="${1:?build folder}"
mkdir -p "$out"
cc="${CC:-gcc -m32}"
$cc -shared -fPIC -O2 -Wall -Wextra -Werror -o "$out/libowneet-no-crash-reporter.so" "$here/../no-crash-reporter.c" -ldl
$cc -O2 -Wall -Wextra -Werror -o "$out/game" "$here/game.c"
lib="$out/libowneet-no-crash-reporter.so"
fail() { echo "FAIL: $*" >&2; exit 1; }

mkdir -p "$out/gamedir"
printf '#!/bin/sh\necho reporter ran\n' > "$out/gamedir/CrashReporter"
chmod +x "$out/gamedir/CrashReporter"
status=0
got="$(LD_PRELOAD="$lib" "$out/game" "$out/gamedir/CrashReporter")" || status=$?
[[ "$got" == "" && "$status" == 0 ]] || fail "the crash reporter was started ($got, $status)"
[[ "$("$out/game" "$out/gamedir/CrashReporter")" == "reporter ran" ]] || fail "without the library the reporter should run"
[[ "$(LD_PRELOAD="$lib" "$out/game" /usr/bin/true; echo "status $?")" == "status 0" ]] || fail "another program was not started"
echo "no-crash-reporter: all checks passed"
