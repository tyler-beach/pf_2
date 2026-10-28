package pathfinder_pr_scan_e2e_fail

import "os/exec"

// Intentional Pathfinder FAIL for same-repo PR-scan E2E (REQ-003-01).
// Lives under internal/ so live-catalog source_path_prefixes apply.
// Product analyzers must never copy this pattern.
func RunUserCommand() error {
	cmd := exec.Command("sh", "-c", "echo intentional-pathfinder-pr-scan-e2e-fail")
	return cmd.Run()
}
