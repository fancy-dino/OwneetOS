// SPDX-License-Identifier: GPL-3.0-or-later
//
// OwneetOS: a game's own crash reporter is not started ([[games-on-disk]], owner 2026-11-12).
// Loaded with LD_PRELOAD only in the games the list of known games names (frontend/known-games;
// old-sdl2-launcher hands it to the game its launcher starts). The Witcher 2 crashes when it is
// quit from its menu; its crash handler (Breakpad) then runs `execv` on the `CrashReporter` next
// to the game, a 2015 program that would send the report to CD Projekt: today it cannot start,
// and its script shows the error in a desktop window (zenity) only a mouse closes. The reporter's
// process ends at once instead: the game ends, the console's home screen comes back, nothing is
// sent.

#define _GNU_SOURCE
#include <dlfcn.h>
#include <string.h>
#include <unistd.h>

#define REPORTER "CrashReporter"

int execv(const char *path, char *const argv[])
{
    static int (*real_execv)(const char *, char *const[]);
    const char *base = strrchr(path, '/');
    if (strcmp(base ? base + 1 : path, REPORTER) == 0)
        _exit(0);
    if (!real_execv)
        real_execv = (int (*)(const char *, char *const[]))dlsym(RTLD_NEXT, "execv");
    return real_execv(path, argv);
}
