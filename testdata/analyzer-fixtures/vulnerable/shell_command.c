#include <stdlib.h>
int main(void) {
    const char *user_input = getenv("USER_INPUT");
    return system(user_input);
}
