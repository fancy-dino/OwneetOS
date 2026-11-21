// SPDX-License-Identifier: GPL-3.0-or-later
// The SDL3-like behaviour the library works around: waits return at once, events come from
// FAKE_EVENT, spawning always works.
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

typedef union { uint32_t type; uint8_t padding[56]; } Event;

int SDL_WaitEventTimeout(Event *event, int timeout) { (void)event; (void)timeout; return 0; }

void SDL_Delay(uint32_t ms)
{
    struct timespec t = { ms / 1000, (long)(ms % 1000) * 1000000L };
    nanosleep(&t, NULL);
}

// FAKE_EVENTS: "type:byte12:value16,..." handed out one per call, then none
int fake_controllers_opened;

int SDL_PollEvent(Event *event)
{
    static const char *next;
    static int started;
    if (!started) {
        next = getenv("FAKE_EVENTS");
        started = 1;
    }
    if (!next || !*next)
        return 0;
    char *end;
    uint32_t type = (uint32_t)strtoul(next, &end, 0);
    unsigned long byte12 = *end == ':' ? strtoul(end + 1, &end, 0) : 0;
    long value = *end == ':' ? strtol(end + 1, &end, 0) : 0;
    next = *end == ',' ? end + 1 : end;
    if (event) {
        memset(event, 0, sizeof *event);
        event->type = type;
        event->padding[12] = (uint8_t)byte12;
        int16_t v = (int16_t)value;
        memcpy(event->padding + 16, &v, 2);
    }
    return 1;
}

void *SDL_JoystickOpen(int index) { static int j; (void)index; return &j; }
void *SDL_GameControllerOpen(int index) { static int c; (void)index; fake_controllers_opened++; return &c; }

int g_spawn_command_line_async(const char *command_line, void **error) { (void)command_line; (void)error; return 1; }
