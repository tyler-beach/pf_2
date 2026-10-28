package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// commandContext returns a cancellable root context bound to SIGINT/SIGTERM.
// Long-running analyzer and assist commands should take this context so
// cancellation propagates into plan.ExecuteManifest and HTTP clients
// (Go blog: Contexts and structs; Effective Go: concurrency cancellation).
func commandContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
