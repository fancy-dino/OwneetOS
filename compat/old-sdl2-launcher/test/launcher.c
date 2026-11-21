// SPDX-License-Identifier: GPL-3.0-or-later
// A program shaped like the launcher: `wait` times one short wait, `poll` polls one event,
// `spawn` starts the game, `env` prints LD_PRELOAD. It prints "went on" when it was not ended.
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

typedef union { uint32_t type; uint8_t padding[56]; } Event;
int SDL_WaitEventTimeout(Event *event, int timeout);
int SDL_PollEvent(Event *event);
int g_spawn_command_line_async(const char *command_line, void **error);
void *SDL_JoystickOpen(int index);
void *SDL_GameControllerOpen(int index);
extern int fake_controllers_opened;
void _ZdlPv(void *block); // operator delete(void*)

int main(int argc, char **argv)
{
    if (argc < 2)
        return 2;
    if (strcmp(argv[1], "wait") == 0) {
        struct timespec a, b;
        clock_gettime(CLOCK_MONOTONIC, &a);
        SDL_WaitEventTimeout(NULL, 1);
        clock_gettime(CLOCK_MONOTONIC, &b);
        printf("%ld\n", (b.tv_sec - a.tv_sec) * 1000 + (b.tv_nsec - a.tv_nsec) / 1000000);
    } else if (strcmp(argv[1], "poll") == 0) {
        // every event the launcher would get: keys as "down:SYM" / "up:SYM", others as their type
        Event e;
        while (SDL_PollEvent(&e)) {
            int32_t sym;
            memcpy(&sym, e.padding + 20, 4);
            if (e.type == 0x300 || e.type == 0x301)
                printf("%s:%d ", e.type == 0x300 ? "down" : "up", sym & ~(1 << 30));
            else
                printf("0x%x ", e.type);
        }
        printf("opened=%d\n", fake_controllers_opened);
        return 0;
    } else if (strcmp(argv[1], "open") == 0) {
        printf("joystick=%s controller=%s\n", SDL_JoystickOpen(0) ? "open" : "none",
               SDL_GameControllerOpen(0) ? "open" : "none");
        return 0;
    } else if (strcmp(argv[1], "delete") == 0) {
        // a 24-byte block, like a std::map node, read after it is deleted (the launcher's bug)
        volatile uint32_t *node = malloc(24);
        for (int i = 0; i < 6; i++)
            node[i] = 0x11111111u * (uint32_t)(i + 1);
        _ZdlPv((void *)node);
        printf("%s\n", node[1] == 0x22222222u && node[3] == 0x44444444u ? "kept" : "changed");
        return 0;
    } else if (strcmp(argv[1], "spawn") == 0) {
        g_spawn_command_line_async("./game", NULL);
    } else if (strcmp(argv[1], "env") == 0) {
        const char *p = getenv("LD_PRELOAD");
        printf("[%s]\n", p ? p : "");
        return 0;
    }
    printf("went on\n");
    return 0;
}
