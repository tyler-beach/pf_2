package main

import (
	"context"
	"testing"
	"time"
)

func TestCommandContextCancelStopsWaiters(t *testing.T) {
	ctx, stop := commandContext()
	defer stop()

	stop()
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != context.Canceled {
			t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("commandContext did not cancel after stop()")
	}
}
