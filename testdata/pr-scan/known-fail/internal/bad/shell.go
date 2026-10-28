package bad

import "os/exec"

// Intentional Pathfinder FAIL fixture for PR-scan E2E (REQ-003-01 /
// pathfinder.go-shell-command). Product analyzers must never copy this pattern:
// they resolve an absolute binary, reject shell interpreters, validate argv,
// optionally VerifyBinary/SHA-256, then exec.CommandContext only after checks
// (see internal/analyzer/command.go).
//
// The -c payload stays a string literal so Semgrep CWE-94 (non-static Command)
// does not treat this testdata anti-pattern as a product injection finding.
func RunUserCommand() error {
	cmd := exec.Command("sh", "-c", "echo intentional-pathfinder-fail-fixture")
	return cmd.Run()
}

// e2e marker: pathfinder-pr-scan known-FAIL fixture (2026-10-28T12:53:02Z)
