// SPDX-License-Identifier: GPL-3.0-or-later
// Runs `execv` on argv[1] with argv[1] as its only argument; prints "went on" if it came back.
#include <stdio.h>
#include <unistd.h>

int main(int argc, char **argv)
{
    if (argc < 2)
        return 2;
    char *args[] = { argv[1], NULL };
    execv(argv[1], args);
    printf("went on\n");
    return 3;
}
