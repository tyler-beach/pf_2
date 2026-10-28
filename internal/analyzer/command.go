package analyzer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CommandResult keeps the exit code separate from launch/cancellation errors.
// OpenGrep may use exit code 1 to report findings; other non-zero codes are
// treated as execution failures.
type CommandResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// CommandRunner is injectable so command construction, cancellation, and exit
// handling can be tested without installing or executing an analyzer binary.
type CommandRunner interface {
	Run(ctx context.Context, binary string, args []string, dir string) (CommandResult, error)
}

// ExecCommandRunner launches analyzer subprocesses without a shell and with a
// credential-free environment. Callers must supply a path that has already
// passed ResolveExecutable / VerifyBinary (or a name that LookPath can pin to
// an absolute regular file); Run re-resolves and validates before exec.
type ExecCommandRunner struct {
	// HomeDir is an isolated scratch directory for configuration/cache lookups.
	// Scanners create and remove it around each scan.
	HomeDir string
}

func (r ExecCommandRunner) Run(ctx context.Context, binary string, args []string, dir string) (CommandResult, error) {
	trustedPath, trustedArgs, err := prepareTrustedAnalyzerCommand(binary, args)
	if err != nil {
		return CommandResult{ExitCode: -1}, err
	}

	// trustedPath is absolute and symlink-resolved to a regular file after
	// ResolveExecutable; args are validated to reject shell wrappers. The
	// binary cannot be a compile-time string literal because scanners share
	// this runner across allowlisted analyzer executables (opengrep/checkov)
	// whose pinned paths are chosen only after VerifyBinary / ResolveExecutable.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, trustedPath, trustedArgs...)
	cmd.Dir = dir
	cmd.Env = scannerEnvironment()
	if r.HomeDir != "" {
		cmd.Env = append(cmd.Env, "HOME="+r.HomeDir)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		result.ExitCode = 0
		return result, nil
	}
	if ctx.Err() != nil {
		result.ExitCode = -1
		return result, fmt.Errorf("analyzer command cancelled: %w; stderr: %s", ctx.Err(), truncate(stderr.String(), 4096))
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	result.ExitCode = -1
	return result, fmt.Errorf("could not start analyzer %q: %w; stderr: %s", trustedPath, err, truncate(stderr.String(), 4096))
}

// ResolveExecutable finds an executable through PATH when necessary and
// resolves symlinks to the immutable file whose bytes will be hashed/executed.
func ResolveExecutable(binary string) (string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return "", fmt.Errorf("analyzer executable path is required")
	}
	if strings.IndexByte(binary, 0) >= 0 {
		return "", fmt.Errorf("analyzer executable path contains NUL")
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("resolve analyzer executable: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve analyzer executable path: %w", err)
	}
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve analyzer executable symlinks: %w", err)
	}
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("analyzer executable path must be absolute after resolution")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect analyzer executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("analyzer executable is not a regular file")
	}
	if err := rejectShellExecutable(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// VerifyBinary resolves an analyzer executable and, when expectedSHA256 is set,
// fails closed unless the file digest matches. The returned path is absolute
// and safe to pass to ExecCommandRunner.
func VerifyBinary(binary, expectedSHA256 string) (path string, actualSHA256 string, err error) {
	resolved, err := ResolveExecutable(binary)
	if err != nil {
		return "", "", err
	}
	actual, err := HashFileCached(resolved)
	if err != nil {
		return "", "", fmt.Errorf("hash analyzer executable: %w", err)
	}
	expected := strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expected == "" {
		return resolved, actual, nil
	}
	if !validPlanSHA256(expected) {
		return "", "", fmt.Errorf("expected analyzer binary SHA-256 is invalid")
	}
	if actual != expected {
		return "", "", fmt.Errorf("analyzer binary SHA-256 mismatch: expected %s but found %s", expected, actual)
	}
	return resolved, actual, nil
}

func prepareTrustedAnalyzerCommand(binary string, args []string) (string, []string, error) {
	resolved, err := ResolveExecutable(binary)
	if err != nil {
		return "", nil, err
	}
	if err := validateAnalyzerArgs(args); err != nil {
		return "", nil, err
	}
	return resolved, append([]string(nil), args...), nil
}

func validateAnalyzerArgs(args []string) error {
	for _, arg := range args {
		if strings.IndexByte(arg, 0) >= 0 {
			return fmt.Errorf("analyzer argument contains NUL")
		}
		// Analyzers are invoked with discrete argv; never accept shell
		// command-string flags that would turn argv into code execution.
		switch arg {
		case "-c", "/c", "-Command", "-EncodedCommand":
			return fmt.Errorf("refusing shell command-string analyzer arguments")
		}
	}
	return nil
}

func rejectShellExecutable(path string) error {
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "sh", "bash", "zsh", "csh", "tcsh", "ksh", "dash", "fish",
		"cmd", "powershell", "pwsh", "env":
		return fmt.Errorf("refusing to execute shell interpreter %q as analyzer", path)
	}
	return nil
}

// scannerEnvironment deliberately excludes ambient credentials such as
// GITHUB_TOKEN, cloud credentials, and API keys from analyzer subprocesses.
func scannerEnvironment() []string {
	var env []string
	for _, name := range []string{"PATH", "TMPDIR", "TEMP", "TMP", "SystemRoot", "WINDIR"} {
		if value := os.Getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	env = append(env, "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "NO_COLOR=1")
	sort.Strings(env)
	return env
}
