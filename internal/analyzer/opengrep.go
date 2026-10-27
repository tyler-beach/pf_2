package analyzer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, binary string, args []string, dir string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = scannerEnvironment()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
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
	return result, fmt.Errorf("could not start analyzer %q: %w; stderr: %s", binary, err, truncate(stderr.String(), 4096))
}

// scannerEnvironment deliberately excludes ambient credentials such as
// GITHUB_TOKEN, cloud credentials, and API keys from analyzer subprocesses.
func scannerEnvironment() []string {
	var env []string
	for _, name := range []string{"PATH", "HOME", "TMPDIR", "TEMP", "TMP", "SystemRoot", "WINDIR", "USERPROFILE"} {
		if value := os.Getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	env = append(env, "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "NO_COLOR=1")
	sort.Strings(env)
	return env
}

// OpenGrepScanner runs a version-pinned OpenGrep CLI and normalizes SARIF 2.1.0.
// ConfigPath must point to a rule file inside Root; the scanner is run without a
// shell and receives only a small, credential-free environment.
type OpenGrepScanner struct {
	Binary     string
	Version    string
	ConfigPath string
	Timeout    time.Duration
	Runner     CommandRunner
}

func (s OpenGrepScanner) Scan(ctx context.Context, root string) (ScanResult, error) {
	out := ScanResult{
		SchemaVersion: "1",
		SARIFVersion: SARIFVersion,
		Status: ScanStatusError,
		Analyzers: []AnalyzerInfo{{Name: "opengrep", Version: s.Version}},
		Findings: []Finding{},
		Errors: []string{},
	}
	if strings.TrimSpace(s.Version) == "" {
		return out, fmt.Errorf("OpenGrep version must be explicitly pinned")
	}
	if strings.TrimSpace(s.ConfigPath) == "" {
		return out, fmt.Errorf("OpenGrep ConfigPath is required")
	}
	binary := strings.TrimSpace(s.Binary)
	if binary == "" { binary = "opengrep" }
	if root == "" { root = "." }
	rootAbs, err := filepath.Abs(root)
	if err != nil { return out, fmt.Errorf("resolve repository root: %w", err) }
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil { return out, fmt.Errorf("resolve repository root symlinks: %w", err) }
	rootInfo, err := os.Stat(rootAbs)
	if err != nil { return out, fmt.Errorf("inspect repository root: %w", err) }
	if !rootInfo.IsDir() { return out, fmt.Errorf("repository root %q is not a directory", rootAbs) }

	configAbs, configRel, err := resolveConfig(rootAbs, s.ConfigPath)
	if err != nil { return out, err }
	configHash, err := HashFile(configAbs)
	if err != nil { return out, err }
	inputHash, err := hashInputTree(rootAbs)
	if err != nil { return out, err }
	out.ConfigurationSHA256 = configHash
	out.InputSHA256 = inputHash

	runner := s.Runner
	if runner == nil { runner = ExecCommandRunner{} }
	runCtx := ctx
	cancel := func() {}
	if s.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.Timeout)
	}
	defer cancel()

	versionResult, err := runner.Run(runCtx, binary, []string{"--version"}, rootAbs)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	if versionResult.ExitCode != 0 {
		err := fmt.Errorf("OpenGrep version probe exited %d: %s", versionResult.ExitCode, truncate(string(versionResult.Stderr), 4096))
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	versionOutput := strings.TrimSpace(string(versionResult.Stdout) + " " + string(versionResult.Stderr))
	if !strings.Contains(versionOutput, s.Version) {
		err := fmt.Errorf("OpenGrep version mismatch: expected %q, command reported %q", s.Version, truncate(versionOutput, 512))
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}

	tempDir, err := os.MkdirTemp("", "pathfinder-opengrep-")
	if err != nil { return out, fmt.Errorf("create temporary scan directory: %w", err) }
	defer os.RemoveAll(tempDir)
	sarifPath := filepath.Join(tempDir, "results.sarif")
	args := []string{"scan", "--sarif-output=" + sarifPath, "-f", configRel, "."}
	commandResult, err := runner.Run(runCtx, binary, args, rootAbs)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	if commandResult.ExitCode != 0 && commandResult.ExitCode != 1 {
		err := fmt.Errorf("OpenGrep scan exited %d: %s", commandResult.ExitCode, truncate(string(commandResult.Stderr), 4096))
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	raw, err := os.ReadFile(sarifPath)
	if err != nil {
		wrapped := fmt.Errorf("read OpenGrep SARIF output: %w", err)
		out.Errors = append(out.Errors, wrapped.Error())
		return out, wrapped
	}
	normalized, err := ParseSARIF(raw, rootAbs)
	if err != nil {
		wrapped := fmt.Errorf("normalize OpenGrep SARIF: %w", err)
		out.Errors = append(out.Errors, wrapped.Error())
		return out, wrapped
	}
	if len(normalized.Analyzers) == 0 || !strings.EqualFold(normalized.Analyzers[0].Name, "opengrep") {
		return out, fmt.Errorf("OpenGrep returned unexpected SARIF tool metadata: %+v", normalized.Analyzers)
	}
	normalized.ConfigurationSHA256 = configHash
	normalized.InputSHA256 = inputHash
	if normalized.Status != ScanStatusComplete {
		return normalized, fmt.Errorf("OpenGrep scan was incomplete; refusing to treat partial evidence as complete")
	}
	if commandResult.ExitCode == 1 && len(normalized.Findings) == 0 {
		normalized.Status = ScanStatusError
		normalized.Errors = append(normalized.Errors, "OpenGrep exited 1 without any reported findings")
		return normalized, fmt.Errorf("OpenGrep exited 1 but produced no findings")
	}
	return normalized, nil
}

func resolveConfig(root, config string) (absolute, relative string, err error) {
	candidate := config
	if !filepath.IsAbs(candidate) { candidate = filepath.Join(root, candidate) }
	candidate, err = filepath.Abs(candidate)
	if err != nil { return "", "", fmt.Errorf("resolve OpenGrep configuration path: %w", err) }
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil { return "", "", fmt.Errorf("resolve OpenGrep configuration symlinks: %w", err) }
	rel, err := filepath.Rel(root, candidate)
	if err != nil { return "", "", fmt.Errorf("make OpenGrep config repository-relative: %w", err) }
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("OpenGrep config path must remain inside repository root")
	}
	info, err := os.Stat(candidate)
	if err != nil { return "", "", fmt.Errorf("inspect OpenGrep configuration: %w", err) }
	if !info.Mode().IsRegular() { return "", "", fmt.Errorf("OpenGrep ConfigPath must be a regular file") }
	return candidate, filepath.ToSlash(rel), nil
}

// hashInputTree hashes relative paths and contents in stable lexical order. Git
// internals and dependency/vendor trees are excluded; symlinks are not followed.
func hashInputTree(root string) (string, error) {
	h := sha256.New()
	skipDir := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".opengrep-cache": true}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if entry.IsDir() {
			if current != root && skipDir[entry.Name()] { return filepath.SkipDir }
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 { return nil }
		info, err := entry.Info()
		if err != nil { return err }
		if !info.Mode().IsRegular() { return nil }
		relative, err := filepath.Rel(root, current)
		if err != nil { return err }
		if _, err := io.WriteString(h, filepath.ToSlash(relative)+"\x00"); err != nil { return err }
		file, err := os.Open(current)
		if err != nil { return err }
		_, copyErr := io.Copy(h, file)
		closeErr := file.Close()
		if copyErr != nil { return copyErr }
		if closeErr != nil { return closeErr }
		if _, err := io.WriteString(h, "\x00"); err != nil { return err }
		return nil
	})
	if err != nil { return "", fmt.Errorf("hash repository input tree: %w", err) }
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeHashPart(h hash.Hash, data []byte) error {
	_, err := h.Write(data)
	return err
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max { return value }
	return value[:max] + "…"
}
