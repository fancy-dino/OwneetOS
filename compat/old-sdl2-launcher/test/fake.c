// SPDX-License-Identifier: GPL-3.0-or-later
// The SDL3-like behaviour the library works around: waits return at once, events come from
// FAKE_EVENT, spawning always works.
#include <stdint.h>
#include <stdlib.h>
#include <time.h>

typedef union { uint32_t type; uint8_t padding[56]; } Event;

int SDL_WaitEventTimeout(Event *event, int timeout) { (void)event; (void)timeout; return 0; }

void SDL_Delay(uint32_t ms)
{
    struct timespec t = { ms / 1000, (long)(ms % 1000) * 1000000L };
    nanosleep(&t, NULL);
}

int SDL_PollEvent(Event *event)
{
    const char *e = getenv("FAKE_EVENT");
    if (!e)
        return 0;
    if (event)
        event->type = (uint32_t)strtoul(e, NULL, 0);
    return 1;
}

int g_spawn_command_line_async(const char *command_line, void **error) { (void)command_line; (void)error; return 1; }
