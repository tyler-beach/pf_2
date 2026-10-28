package main

import (
	"os"
	"strings"

	"github.com/tyler-beach/pf_2/internal/orchestrate"
)

// cliObserver enables JSON stage logs when PATHFINDER_LOG is set to json|1|true|slog.
// Quiet by default so CI stdout/stderr contracts stay stable unless operators opt in.
func cliObserver() orchestrate.Observer {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PATHFINDER_LOG"))) {
	case "1", "true", "json", "slog", "on":
		return orchestrate.DefaultObserver()
	default:
		return orchestrate.NopObserver{}
	}
}
