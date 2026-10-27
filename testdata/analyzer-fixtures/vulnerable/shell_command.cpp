#include <cstdlib>
int main() {
    const char *user_input = std::getenv("USER_INPUT");
    return std::system(user_input);
}
