package main

import "os/exec"

func run(command string) {
  exec.Command("sh", "-c", command) // reviewdog smoke: keep finding on a changed line
}
