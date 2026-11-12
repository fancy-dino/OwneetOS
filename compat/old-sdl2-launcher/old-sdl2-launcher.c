// SPDX-License-Identifier: GPL-3.0-or-later
//
// OwneetOS: what old launchers made for SDL2 expected from it, under SDL3 (sdl2-compat)
// ([[games-on-disk]], owner 2026-11-12, open-items 3u). Loaded with LD_PRELOAD, only for the
// files the list of known games names (frontend/known-games): The Witcher 2's TV launcher.
//
// 1. SDL_WaitEventTimeout with a timeout under 10 ms waits 10 ms when no event comes, as SDL2
//    did (it polled every 10 ms). The launcher updates its screen only when one turn of its loop
//    lasts more than 10 ms; under SDL3 a turn lasts 1 ms, so its screen stayed black.
// 2. Once the launcher has started the game (g_spawn_command_line_async), it ends at once: its
//    own clean-up then crashes (a std::map destroyed twice, stack overflow), and its settings are
//    saved before.
// 3. A request to quit (SDL_QUIT: the console closing the game sends SIGTERM) ends it at once:
//    the launcher logs it and ignores it, so it was killed after the stop timeout.
//
// The library takes itself out of LD_PRELOAD when it loads, so the game the launcher starts does
// not get it.

#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define SDL_QUIT_EVENT 0x100
#define OLD_SDL2_POLL_MS 10

typedef union {
    uint32_t type;
    uint8_t padding[56]; // SDL2's SDL_Event
} Event;
typedef struct GError GError;

static void *next_symbol(const char *name)
{
    return dlsym(RTLD_NEXT, name);
}

// LD_PRELOAD without this library (entries are separated by ':' or ' ')
__attribute__((constructor)) static void leave_ld_preload(void)
{
    Dl_info self;
    const char *preload = getenv("LD_PRELOAD");
    if (!preload || !dladdr((void *)leave_ld_preload, &self) || !self.dli_fname)
        return;
    const char *own = strrchr(self.dli_fname, '/');
    own = own ? own + 1 : self.dli_fname;
    char *copy = strdup(preload);
    char *kept = calloc(strlen(preload) + 1, 1);
    if (!copy || !kept) {
        free(copy);
        free(kept);
        return;
    }
    for (char *save = NULL, *entry = strtok_r(copy, ": ", &save); entry; entry = strtok_r(NULL, ": ", &save)) {
        const char *base = strrchr(entry, '/');
        if (strcmp(base ? base + 1 : entry, own) == 0)
            continue;
        if (*kept)
            strcat(kept, ":");
        strcat(kept, entry);
    }
    if (*kept)
        setenv("LD_PRELOAD", kept, 1);
    else
        unsetenv("LD_PRELOAD");
    free(copy);
    free(kept);
}

int SDL_WaitEventTimeout(Event *event, int timeout)
{
    static int (*wait_event)(Event *, int);
    static void (*delay)(uint32_t);
    if (!wait_event) {
        wait_event = (int (*)(Event *, int))next_symbol("SDL_WaitEventTimeout");
        delay = (void (*)(uint32_t))next_symbol("SDL_Delay");
    }
    if (timeout <= 0 || timeout >= OLD_SDL2_POLL_MS || !delay)
        return wait_event(event, timeout);
    int got = wait_event(event, 0);
    if (got)
        return got;
    delay(OLD_SDL2_POLL_MS);
    return wait_event(event, 0);
}

int SDL_PollEvent(Event *event)
{
    static int (*poll_event)(Event *);
    if (!poll_event)
        poll_event = (int (*)(Event *))next_symbol("SDL_PollEvent");
    int got = poll_event(event);
    if (got && event && event->type == SDL_QUIT_EVENT)
        _exit(0);
    return got;
}

int g_spawn_command_line_async(const char *command_line, GError **error)
{
    static int (*spawn)(const char *, GError **);
    if (!spawn)
        spawn = (int (*)(const char *, GError **))next_symbol("g_spawn_command_line_async");
    int started = spawn(command_line, error);
    if (started)
        _exit(0);
    return started;
}
