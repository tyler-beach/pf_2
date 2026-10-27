object Main {
  def main(args: Array[String]): Unit = {
    val command = sys.env.getOrElse("USER_COMMAND", "true")
    Runtime.getRuntime().exec(command)
  }
}
