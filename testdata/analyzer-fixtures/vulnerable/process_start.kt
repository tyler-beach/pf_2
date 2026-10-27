fun main() {
    val command = System.getenv("USER_COMMAND")
    Runtime.getRuntime().exec(command)
}
