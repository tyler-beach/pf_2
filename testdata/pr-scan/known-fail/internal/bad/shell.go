package bad

import "os/exec"

// Intentional Pathfinder FAIL fixture: dynamic shell composition for PR-scan E2E.
func RunUserCommand(userInput string) error {
	cmd := exec.Command("sh", "-c", "echo "+userInput)
	return cmd.Run()
}
