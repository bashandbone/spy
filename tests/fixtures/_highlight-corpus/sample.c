/* C sample */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    int value;
} Counter;

static int counter_increment(Counter *c, int n) {
    c->value += n;
    return c->value;
}

int main(void) {
    FILE *f = fopen("input.txt", "r");
    if (f == NULL) {
        perror("fopen");
        return 1;
    }
    Counter c = {0};
    char buf[4096];
    while (fgets(buf, sizeof(buf), f) != NULL) {
        size_t n = strlen(buf);
        if (n > 0 && buf[n - 1] == '\n') {
            n--;
        }
        if (n > 0) {
            counter_increment(&c, (int)n);
        }
    }
    fclose(f);
    printf("total bytes: %d\n", c.value);
    return 0;
}
