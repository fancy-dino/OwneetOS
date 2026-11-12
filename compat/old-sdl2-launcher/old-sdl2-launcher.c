// SPDX-License-Identifier: GPL-3.0-or-later
//
// OwneetOS: what old launchers made for SDL2 expected from it, under SDL3 (sdl2-compat)
// ([[games-on-disk]], owner 2026-11-12, open-items 3u). Loaded with LD_PRELOAD, only for the
// files the list of known games names (frontend/known-games): The Witcher 2's TV launcher.
//
// 1. SDL_WaitEventTimeout with a timeout under 10 ms waits 10 ms when no event comes, as SDL2
//    did (it polled every 10 ms). The launcher updates its screen only when one turn of its loop
//    lasts more than 10 ms; under SDL3 a turn lasts 1 ms, so its screen stayed black.
// 2. Once the launcher has started the game (g_spawn_command_line_async), it ends at once (its
//    settings are saved before).
// 3. A request to quit (SDL_QUIT: the console closing the game sends SIGTERM) ends it at once:
//    the launcher logs it and ignores it, so it was killed after the stop timeout.
// 4. Memory the launcher deletes is kept as it was (operator delete does not free it): the
//    launcher reads a std::map's node after erasing it (found with valgrind); today's glibc writes
//    into freed blocks at once, which corrupted its maps and crashed it (stack overflow in a tree
//    walk) while the options were changed or after Start Game. The launcher lives a few minutes.
// 5. The controller is turned into its keys (owner, 2026-11-12): its own controller handling is
//    broken under SDL3 (B changed values instead of going back) and it ignores the mouse, while its
//    keyboard works (arrows move and change values, Enter opens, Esc goes back). The library opens
//    the controllers itself (SDL's own mapping) and hands the launcher keyboard events: D-pad and
//    left stick = arrows, A = Enter, B = Esc; the launcher never sees a joystick or controller, so a
//    button never means two things on one screen.
//
// The library takes itself out of LD_PRELOAD when it loads, so the game the launcher starts does
// not get it; OWNEET_GAME_PRELOAD, when set, takes its place: the library the game gets
// (no-crash-reporter, owner 2026-11-12).

#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// SDL2's event types, controller buttons and axes, keys
#define SDL_QUIT_EVENT 0x100
#define SDL_KEYDOWN_EVENT 0x300
#define SDL_KEYUP_EVENT 0x301
#define SDL_JOYSTICK_EVENTS_FIRST 0x600 // joystick and controller events
#define SDL_JOYSTICK_EVENTS_LAST 0x6ff
#define SDL_CONTROLLERAXISMOTION 0x650
#define SDL_CONTROLLERBUTTONDOWN 0x651
#define SDL_CONTROLLERBUTTONUP 0x652
#define SDL_CONTROLLERDEVICEADDED 0x653
enum { BUTTON_A = 0, BUTTON_B = 1, DPAD_UP = 11, DPAD_DOWN = 12, DPAD_LEFT = 13, DPAD_RIGHT = 14 };
enum { AXIS_LEFTX = 0, AXIS_LEFTY = 1 };
enum { KEY_RETURN, KEY_ESCAPE, KEY_RIGHT, KEY_LEFT, KEY_DOWN, KEY_UP, KEY_NONE };
static const struct { int32_t scancode, sym; } keys[] = {
    { 40, 13 }, { 41, 27 }, // Return, Escape
    { 79, (1 << 30) | 79 }, { 80, (1 << 30) | 80 }, { 81, (1 << 30) | 81 }, { 82, (1 << 30) | 82 },
};
#define STICK_PRESS 16000 // of 32767: a stick pushed past half is an arrow key held
#define STICK_RELEASE 8000
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

// LD_PRELOAD without this library (entries are separated by ':' or ' '), with the game's own
// library (OWNEET_GAME_PRELOAD) instead
__attribute__((constructor)) static void leave_ld_preload(void)
{
    Dl_info self;
    const char *preload = getenv("LD_PRELOAD");
    const char *game = getenv("OWNEET_GAME_PRELOAD");
    if (!preload || !dladdr((void *)leave_ld_preload, &self) || !self.dli_fname)
        return;
    const char *own = strrchr(self.dli_fname, '/');
    own = own ? own + 1 : self.dli_fname;
    char *copy = strdup(preload);
    char *kept = calloc(strlen(preload) + (game ? strlen(game) : 0) + 2, 1);
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
    if (game && *game) {
        if (*kept)
            strcat(kept, ":");
        strcat(kept, game);
    }
    unsetenv("OWNEET_GAME_PRELOAD");
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

static int controller_event(const Event *event)
{
    return event->type >= SDL_JOYSTICK_EVENTS_FIRST && event->type <= SDL_JOYSTICK_EVENTS_LAST;
}

// Keyboard events waiting to be handed over (a stick turning may release one arrow and press another)
static Event pending[4];
static int pending_count;

static void queue_key(int key, int down, uint32_t timestamp)
{
    if (key == KEY_NONE || pending_count == (int)(sizeof pending / sizeof pending[0]))
        return;
    Event *e = &pending[pending_count++];
    memset(e, 0, sizeof *e);
    uint8_t *b = e->padding;
    uint32_t type = down ? SDL_KEYDOWN_EVENT : SDL_KEYUP_EVENT;
    memcpy(b + 0, &type, 4);         // SDL_KeyboardEvent: type, timestamp, windowID,
    memcpy(b + 4, &timestamp, 4);    // state, repeat, keysym { scancode, sym, mod }
    b[12] = down ? 1 : 0;
    memcpy(b + 16, &keys[key].scancode, 4);
    memcpy(b + 20, &keys[key].sym, 4);
}

static int button_key(uint8_t button)
{
    switch (button) {
    case BUTTON_A: return KEY_RETURN;
    case BUTTON_B: return KEY_ESCAPE;
    case DPAD_UP: return KEY_UP;
    case DPAD_DOWN: return KEY_DOWN;
    case DPAD_LEFT: return KEY_LEFT;
    case DPAD_RIGHT: return KEY_RIGHT;
    default: return KEY_NONE;
    }
}

// The left stick: the arrow key it holds on each axis (KEY_NONE when centred)
static int stick_key[2] = { KEY_NONE, KEY_NONE };

static void stick_moved(uint8_t axis, int16_t value, uint32_t timestamp)
{
    if (axis != AXIS_LEFTX && axis != AXIS_LEFTY)
        return;
    int held = stick_key[axis];
    int now = held;
    if (value <= -STICK_PRESS)
        now = axis == AXIS_LEFTX ? KEY_LEFT : KEY_UP;
    else if (value >= STICK_PRESS)
        now = axis == AXIS_LEFTX ? KEY_RIGHT : KEY_DOWN;
    else if (value > -STICK_RELEASE && value < STICK_RELEASE)
        now = KEY_NONE;
    if (now == held)
        return;
    if (held != KEY_NONE)
        queue_key(held, 0, timestamp);
    if (now != KEY_NONE)
        queue_key(now, 1, timestamp);
    stick_key[axis] = now;
}

// A controller event becomes keyboard events for the launcher (or nothing)
static void translate(const Event *e)
{
    const uint8_t *b = e->padding;
    uint32_t timestamp;
    memcpy(&timestamp, b + 4, 4);
    switch (e->type) {
    case SDL_CONTROLLERDEVICEADDED: {
        // opened by the library, with SDL's own mapping (the launcher's opening fails)
        static void *(*open_controller)(int);
        if (!open_controller)
            open_controller = (void *(*)(int))next_symbol("SDL_GameControllerOpen");
        int32_t index;
        memcpy(&index, b + 8, 4);
        if (open_controller)
            open_controller(index);
        break;
    }
    case SDL_CONTROLLERBUTTONDOWN:
    case SDL_CONTROLLERBUTTONUP:
        queue_key(button_key(b[12]), e->type == SDL_CONTROLLERBUTTONDOWN, timestamp);
        break;
    case SDL_CONTROLLERAXISMOTION: {
        int16_t value;
        memcpy(&value, b + 16, 2);
        stick_moved(b[12], value, timestamp);
        break;
    }
    }
}

int SDL_PollEvent(Event *event)
{
    static int (*poll_event)(Event *);
    if (!poll_event)
        poll_event = (int (*)(Event *))next_symbol("SDL_PollEvent");
    if (!event) // a peek: SDL's queue is left as it is
        return pending_count > 0 || poll_event(NULL);
    for (;;) {
        if (pending_count > 0) {
            *event = pending[0];
            memmove(pending, pending + 1, (size_t)(pending_count - 1) * sizeof pending[0]);
            pending_count--;
            return 1;
        }
        if (!poll_event(event))
            return 0;
        if (controller_event(event)) {
            translate(event);
            continue;
        }
        if (event->type == SDL_QUIT_EVENT)
            _exit(0);
        return 1;
    }
}

void *SDL_JoystickOpen(int index)
{
    (void)index;
    return NULL;
}

void *SDL_GameControllerOpen(int index)
{
    (void)index;
    return NULL;
}

void _ZdlPv(void *block)
{
    (void)block; // kept: the launcher reads what it has just deleted
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
