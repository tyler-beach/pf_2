package main

import "os/exec"

func run(command string) {
  exec.Command("sh", "-c", command)
}
