package prscan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func knownFailShellFixture(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	return filepath.Join(root, "testdata", "pr-scan", "known-fail", "internal", "bad", "shell.go")
}

func TestKnownFailShellFixtureKeepsPathfinderShellPattern(t *testing.T) {
	body, err := os.ReadFile(knownFailShellFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	if !strings.Contains(src, `exec.Command("sh", "-c"`) {
		t.Fatal("known-FAIL fixture must keep exec.Command(\"sh\", \"-c\", ...) for pathfinder.go-shell-command")
	}
	if strings.Contains(src, `"+`) || strings.Contains(src, "userInput") {
		t.Fatal("known-FAIL fixture must not use non-static Command argv (Semgrep CWE-94)")
	}
}

func TestProductExecRunnerFailClosedAgainstShellFixturePattern(t *testing.T) {
	dir := t.TempDir()
	shell := filepath.Join(dir, "sh")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := analyzer.ResolveExecutable(shell); err == nil || !strings.Contains(err.Error(), "shell interpreter") {
		t.Fatalf("expected shell rejection before exec, got %v", err)
	}
	runner := analyzer.ExecCommandRunner{}
	_, err := runner.Run(context.Background(), shell, []string{"-c", "echo intentional-pathfinder-fail-fixture"}, dir)
	if err == nil {
		t.Fatal("expected fail-closed rejection of shell -c before CommandContext")
	}
	if !strings.Contains(err.Error(), "shell interpreter") && !strings.Contains(err.Error(), "shell command-string") {
		t.Fatalf("unexpected error: %v", err)
	}
}
