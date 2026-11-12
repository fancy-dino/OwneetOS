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
        Event e;
        SDL_PollEvent(&e);
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
