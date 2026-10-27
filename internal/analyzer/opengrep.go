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
	"strconv"
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

type ExecCommandRunner struct {
	// HomeDir is an isolated scratch directory for configuration/cache lookups.
	// OpenGrepScanner creates and removes it around each scan.
	HomeDir string
}

func (r ExecCommandRunner) Run(ctx context.Context, binary string, args []string, dir string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = scannerEnvironment()
	if r.HomeDir != "" { cmd.Env = append(cmd.Env, "HOME="+r.HomeDir) }
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
	for _, name := range []string{"PATH", "TMPDIR", "TEMP", "TMP", "SystemRoot", "WINDIR"} {
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
	out.ConfigurationPath = configRel
	inputHash, err := hashInputTree(rootAbs)
	if err != nil { return out, err }
	out.ConfigurationSHA256 = configHash
	out.InputSHA256 = inputHash

	tempDir, err := os.MkdirTemp("", "pathfinder-opengrep-")
	if err != nil { return out, fmt.Errorf("create temporary scan directory: %w", err) }
	defer os.RemoveAll(tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.Mkdir(homeDir, 0700); err != nil {
		return out, fmt.Errorf("create isolated analyzer home: %w", err)
	}
	runner := s.Runner
	if runner == nil { runner = ExecCommandRunner{HomeDir: homeDir} }
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
	if len(normalized.Analyzers) == 0 || !strings.Contains(strings.ToLower(normalized.Analyzers[0].Name), "opengrep") {
		return out, fmt.Errorf("OpenGrep returned unexpected SARIF tool metadata: %+v", normalized.Analyzers)
	}
	// Some OpenGrep releases emit the driver version in the driver name and
	// leave driver.version empty. The executable version was already checked
	// against the pinned version above; preserve that verified identity.
	for i := range normalized.Analyzers {
		if normalized.Analyzers[i].Version == "" {
			normalized.Analyzers[i].Version = s.Version
		}
	}
	for i := range normalized.Findings {
		if normalized.Findings[i].AnalyzerVersion == "" {
			normalized.Findings[i].AnalyzerVersion = s.Version
		}
	}
	normalized.ConfigurationPath = configRel
	normalized.ConfigurationSHA256 = configHash
	normalized.InputSHA256 = inputHash
	normalized.ScannedPaths, err = discoverScannedPathsExcluding(rootAbs, configRel)
	if err != nil { return out, fmt.Errorf("discover scanner scope: %w", err) }
	if incompleteAnalysisDiagnostic(string(commandResult.Stderr)) {
		normalized.Status = ScanStatusPartial
		normalized.Errors = append(normalized.Errors, "OpenGrep reported incomplete analysis: "+truncate(string(commandResult.Stderr), 4096))
		return normalized, fmt.Errorf("OpenGrep scan was incomplete; refusing to treat partial evidence as complete: %s", truncate(string(commandResult.Stderr), 4096))
	}
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
	skipDir := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".opengrep-cache": true, "analyzer-evidence": true}
	skipFile := map[string]bool{"assessment.json": true, "results.sarif": true, "report.md": true, "coverage.out": true, "ingested-adrs.json": true}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if entry.IsDir() {
			if current != root && skipDir[entry.Name()] { return filepath.SkipDir }
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 { return nil }
		if skipFile[entry.Name()] { return nil }
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


// discoverScannedPaths records a deterministic inventory of source/config files
// that can inform whether a policy's declared repository path scope is present.
func discoverScannedPaths(root string) ([]string, error) {
	return discoverScannedPathsExcluding(root, "")
}

func discoverScannedPathsExcluding(root, excludedPath string) ([]string, error) {
	excludedPath = filepath.ToSlash(filepath.Clean(excludedPath))
	extensions := map[string]bool{
		".go": true, ".py": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
		".java": true, ".cs": true, ".php": true, ".rb": true, ".json": true, ".yaml": true,
		".yml": true, ".tf": true, ".sh": true, ".kt": true, ".kts": true, ".scala": true,
		".rs": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true,
		".vue": true, ".html": true, ".xml": true, ".sql": true, ".ex": true, ".exs": true,
	}
	skipDir := map[string]bool{".git": true, "node_modules": true, "vendor": true, ".opengrep-cache": true, "analyzer-evidence": true}
	var paths []string
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if entry.IsDir() {
			if current != root && skipDir[entry.Name()] { return filepath.SkipDir }
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 { return nil }
		if !entry.Type().IsRegular() && entry.Type() != 0 { return nil }
		if !extensions[strings.ToLower(filepath.Ext(entry.Name()))] { return nil }
		rel, err := filepath.Rel(root, current)
		if err != nil { return err }
		rel = filepath.ToSlash(rel)
		if excludedPath != "" && rel == excludedPath { return nil }
		paths = append(paths, rel)
		return nil
	})
	if err != nil { return nil, err }
	sort.Strings(paths)
	return paths, nil
}


// HashInputTree exposes the deterministic source/configuration tree digest for
// evidence verification. Generated reports and analyzer-evidence outputs are
// excluded so a scan can be verified after its reports are written.
func HashInputTree(root string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil { return "", fmt.Errorf("resolve repository root: %w", err) }
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil { return "", fmt.Errorf("resolve repository root symlinks: %w", err) }
	return hashInputTree(rootAbs)
}


// incompleteAnalysisDiagnostic distinguishes a concrete partial parse/analyzer
// failure from OpenGrep's generic notice that it skipped irrelevant files. That
// summary is common for mixed repositories and is not, by itself, proof that a
// required rule failed to analyze an applicable source.
func incompleteAnalysisDiagnostic(stderr string) bool {
	lower := strings.ToLower(stderr)
	for _, marker := range []string{"rule parse error", "internal opengrep errors", "failed to analyze"} {
		if strings.Contains(lower, marker) { return true }
	}
	const prefix = "partially scanned:"
	if idx := strings.Index(lower, prefix); idx >= 0 {
		remaining := strings.TrimSpace(lower[idx+len(prefix):])
		fields := strings.Fields(remaining)
		if len(fields) == 0 { return true }
		count, err := strconv.Atoi(strings.Trim(fields[0], ":,.;"))
		if err != nil { return true }
		if count > 0 { return true }
	}
	// Do not fail on the broad summary alone. A concrete positive partial-file
	// count, rule parse error, or SARIF invocation error is handled above.
	return false
}
