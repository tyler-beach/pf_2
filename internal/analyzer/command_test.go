package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestVerifyBinaryRejectsDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "fake-analyzer", "#!/bin/sh\nexit 0\n")
	_, _, err := VerifyBinary(binary, strings.Repeat("f", 64))
	if err == nil || !strings.Contains(err.Error(), "binary SHA-256 mismatch") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestVerifyBinaryRejectsInvalidDigest(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "fake-analyzer", "#!/bin/sh\nexit 0\n")
	_, _, err := VerifyBinary(binary, "not-a-digest")
	if err == nil || !strings.Contains(err.Error(), "SHA-256 is invalid") {
		t.Fatalf("expected invalid digest failure, got %v", err)
	}
}

func TestVerifyBinaryAcceptsMatchingDigest(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "fake-analyzer", "#!/bin/sh\nexit 0\n")
	want, err := HashFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	gotPath, gotDigest, err := VerifyBinary(binary, want)
	if err != nil {
		t.Fatalf("VerifyBinary: %v", err)
	}
	if gotPath != binary {
		t.Fatalf("resolved path = %q, want %q", gotPath, binary)
	}
	if gotDigest != want {
		t.Fatalf("digest = %q, want %q", gotDigest, want)
	}
}

func TestResolveExecutableRejectsMissingBinary(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := ResolveExecutable(missing); err == nil {
		t.Fatal("expected missing binary to fail closed")
	}
}

func TestResolveExecutableRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveExecutable(dir)
	if err == nil {
		t.Fatal("expected directory rejection")
	}
	msg := err.Error()
	if !strings.Contains(msg, "regular file") && !strings.Contains(msg, "is a directory") {
		t.Fatalf("expected directory rejection, got %v", err)
	}
}

func TestResolveExecutableRejectsShellInterpreter(t *testing.T) {
	dir := t.TempDir()
	shell := writeExecutable(t, dir, "bash", "#!/bin/sh\nexit 0\n")
	if _, err := ResolveExecutable(shell); err == nil || !strings.Contains(err.Error(), "shell interpreter") {
		t.Fatalf("expected shell rejection, got %v", err)
	}
}

func TestResolveExecutableRejectsSymlinkEscapeToMissingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing-target")
	link := filepath.Join(dir, "broken-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExecutable(link); err == nil {
		t.Fatal("expected broken symlink to fail closed")
	}
}

func TestValidateAnalyzerArgsRejectsShellCommandString(t *testing.T) {
	for _, arg := range []string{"-c", "/c", "-Command", "-EncodedCommand"} {
		if err := validateAnalyzerArgs([]string{arg, "id"}); err == nil {
			t.Fatalf("expected rejection for %q", arg)
		}
	}
	if err := validateAnalyzerArgs([]string{"scan", "--quiet"}); err != nil {
		t.Fatalf("safe args rejected: %v", err)
	}
}

func TestPrepareTrustedAnalyzerCommandRejectsShellArgs(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "fake-analyzer", "#!/bin/sh\nexit 0\n")
	_, _, err := prepareTrustedAnalyzerCommand(binary, []string{"-c", "evil"})
	if err == nil || !strings.Contains(err.Error(), "shell command-string") {
		t.Fatalf("expected shell arg rejection, got %v", err)
	}
}

func TestExecCommandRunnerRejectsUnresolvableBinary(t *testing.T) {
	runner := ExecCommandRunner{}
	missing := filepath.Join(t.TempDir(), "missing-analyzer")
	_, err := runner.Run(context.Background(), missing, []string{"--version"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "resolve analyzer executable") {
		t.Fatalf("expected resolve failure, got %v", err)
	}
}

func TestExecCommandRunnerRejectsShellBinaryBeforeExec(t *testing.T) {
	dir := t.TempDir()
	shell := writeExecutable(t, dir, "sh", "#!/bin/sh\nexit 0\n")
	runner := ExecCommandRunner{}
	_, err := runner.Run(context.Background(), shell, []string{"-c", "true"}, t.TempDir())
	if err == nil {
		t.Fatal("expected shell binary rejection")
	}
	if !strings.Contains(err.Error(), "shell interpreter") && !strings.Contains(err.Error(), "shell command-string") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecCommandRunnerRunsVerifiedAbsoluteBinary(t *testing.T) {
	dir := t.TempDir()
	binary := writeExecutable(t, dir, "fake-analyzer", "#!/bin/sh\necho ok\n")
	digest, err := HashFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	path, actual, err := VerifyBinary(binary, digest)
	if err != nil {
		t.Fatal(err)
	}
	if actual != digest {
		t.Fatalf("digest mismatch after verify: %s vs %s", actual, digest)
	}
	runner := ExecCommandRunner{HomeDir: filepath.Join(dir, "home")}
	result, err := runner.Run(context.Background(), path, []string{}, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr=%s", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(string(result.Stdout), "ok") {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
}
