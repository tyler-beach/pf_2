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
const analyzerFixtureDirectory = "testdata/analyzer-fixtures"

type OpenGrepScanner struct {
	Binary           string
	BinarySHA256     string
	Version          string
	ConfigPath       string
	RuleSetSHA256    string
	SourceExtensions []string
	ScanPaths        []string
	MaxFileBytes     int64
	Timeout          time.Duration
	Runner           CommandRunner
}

// ResolveExecutable finds an executable through PATH when necessary and
// resolves symlinks to the immutable file whose bytes will be hashed/executed.
func ResolveExecutable(binary string) (string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" { return "", fmt.Errorf("analyzer executable path is required") }
	resolved, err := exec.LookPath(binary)
	if err != nil { return "", fmt.Errorf("resolve analyzer executable: %w", err) }
	resolved, err = filepath.Abs(resolved)
	if err != nil { return "", fmt.Errorf("resolve analyzer executable path: %w", err) }
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil { return "", fmt.Errorf("resolve analyzer executable symlinks: %w", err) }
	info, err := os.Stat(resolved)
	if err != nil { return "", fmt.Errorf("inspect analyzer executable: %w", err) }
	if !info.Mode().IsRegular() { return "", fmt.Errorf("analyzer executable is not a regular file") }
	return resolved, nil
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

	binarySHA256 := ""
	if strings.TrimSpace(s.BinarySHA256) != "" {
		expected := strings.ToLower(strings.TrimSpace(s.BinarySHA256))
		if !validPlanSHA256(expected) { return out, fmt.Errorf("expected OpenGrep binary SHA-256 is invalid") }
		resolvedBinary, resolveErr := ResolveExecutable(binary)
		if resolveErr != nil { return out, resolveErr }
		actual, hashErr := HashFile(resolvedBinary)
		if hashErr != nil { return out, fmt.Errorf("hash OpenGrep executable: %w", hashErr) }
		if actual != expected {
			return out, fmt.Errorf("OpenGrep binary SHA-256 mismatch: expected %s but found %s", expected, actual)
		}
		binary, binarySHA256 = resolvedBinary, actual
		out.BinarySHA256 = actual
	}

	configAbs, configRel, err := resolveConfig(rootAbs, s.ConfigPath)
	if err != nil { return out, err }
	configHash, err := HashFile(configAbs)
	if err != nil { return out, err }
	ruleSetHash := strings.ToLower(strings.TrimSpace(s.RuleSetSHA256))
	if ruleSetHash == "" { ruleSetHash = configHash }
	if !validPlanSHA256(ruleSetHash) { return out, fmt.Errorf("OpenGrep rule-set SHA-256 is invalid") }
	if s.MaxFileBytes > 0 {
		if err := validateScanFileSizes(rootAbs, s.MaxFileBytes, s.SourceExtensions); err != nil { return out, err }
	}
	out.ConfigurationPath = configRel
	out.ConfigurationSHA256 = configHash
	out.BinarySHA256 = binarySHA256
	out.RuleSetSHA256 = ruleSetHash
	inputHash, err := hashInputTree(rootAbs)
	if err != nil { return out, err }
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
	selectedPaths, err := normalizeScanPaths(rootAbs, s.ScanPaths, configRel)
	if err != nil { return out, fmt.Errorf("validate requested scan paths: %w", err) }
	args := []string{"scan", "--sarif-output=" + sarifPath, "--exclude", analyzerFixtureDirectory, "-f", configRel}
	if len(selectedPaths) == 0 {
		args = append(args, ".")
	} else {
		for _, selected := range selectedPaths { args = append(args, "./"+filepath.FromSlash(selected)) }
	}
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
	normalized.BinarySHA256 = binarySHA256
	normalized.RuleSetSHA256 = ruleSetHash
	normalized.InputSHA256 = inputHash
	if len(selectedPaths) > 0 {
		normalized.ScannedPaths = append([]string(nil), selectedPaths...)
	} else {
		normalized.ScannedPaths, err = discoverScannedPathsExcluding(rootAbs, configRel)
		if err != nil { return out, fmt.Errorf("discover scanner scope: %w", err) }
	}
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
	skipFile := map[string]bool{"assessment.json": true, "results.sarif": true, "report.md": true, "coverage.out": true, "ingested-adrs.json": true, "obligation-candidates.json": true}
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
			rel, err := filepath.Rel(root, current)
			if err != nil { return err }
			if filepath.ToSlash(rel) == analyzerFixtureDirectory { return filepath.SkipDir }
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 { return nil }
		if !entry.Type().IsRegular() && entry.Type() != 0 { return nil }
		if !extensions[strings.ToLower(filepath.Ext(entry.Name()))] { return nil }
		rel, err := filepath.Rel(root, current)
		if err != nil { return err }
		rel = filepath.ToSlash(rel)
		if excludedPath != "" && rel == excludedPath { return nil }
		if rel == "adr/obligation-candidates.json" || rel == "adr/ingested-adrs.json" { return nil }
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


// validateScanFileSizes fails closed when supported source files exceed the
// configured per-file bound; symlinks and generated analyzer artifacts are
// excluded so scans do not traverse outside the repository or scan their own output.
func validateScanFileSizes(root string, maxBytes int64, sourceExtensions []string) error {
	if maxBytes <= 0 { return nil }
	allowed := make(map[string]bool, len(sourceExtensions))
	for _, ext := range sourceExtensions { allowed[strings.ToLower(strings.TrimSpace(ext))] = true }
	if len(allowed) == 0 { return nil }
	skipDirs := map[string]bool{
		".git": true, "vendor": true, "node_modules": true,
		".opengrep-cache": true, "analyzer-evidence": true,
	}
	return filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if entry.Type()&os.ModeSymlink != 0 { return nil }
		if entry.IsDir() {
			if current != root {
				rel, err := filepath.Rel(root, current)
				if err != nil { return err }
				rel = filepath.ToSlash(rel)
				if skipDirs[entry.Name()] || rel == analyzerFixtureDirectory || strings.HasPrefix(rel, analyzerFixtureDirectory+"/") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !allowed[strings.ToLower(filepath.Ext(entry.Name()))] { return nil }
		info, err := entry.Info()
		if err != nil { return fmt.Errorf("inspect source file size: %w", err) }
		if !info.Mode().IsRegular() { return nil }
		if info.Size() > maxBytes {
			rel, relErr := filepath.Rel(root, current)
			if relErr != nil { return relErr }
			return fmt.Errorf("source file %s is %d bytes and exceeds the configured %d-byte per-file limit", filepath.ToSlash(rel), info.Size(), maxBytes)
		}
		return nil
	})
}

// normalizeScanPaths validates the explicit file inventory used by a scoped
// plan. Paths are repository-relative regular files; symlinks, traversal,
// duplicate entries, and the analyzer configuration itself are rejected.
func normalizeScanPaths(root string, requested []string, configRel string) ([]string, error) {
	if len(requested) == 0 { return nil, nil }
	eligiblePaths, err := discoverScannedPathsExcluding(root, configRel)
	if err != nil { return nil, fmt.Errorf("build eligible source inventory: %w", err) }
	eligible := make(map[string]bool, len(eligiblePaths))
	for _, path := range eligiblePaths { eligible[path] = true }
	seen := make(map[string]bool, len(requested))
	out := make([]string, 0, len(requested))
	for _, raw := range requested {
		item := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(item)))
		if item == "" || clean == "." || clean != item || filepath.IsAbs(filepath.FromSlash(clean)) ||
			(len(clean) >= 2 && clean[1] == ':') || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("scan path %q must be normalized and repository-relative", raw)
		}
		absolute := filepath.Join(root, filepath.FromSlash(clean))
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil { return nil, fmt.Errorf("resolve scan path %q: %w", clean, err) }
		if filepath.Clean(resolved) != filepath.Clean(absolute) {
			return nil, fmt.Errorf("scan path %q traverses a symlink", clean)
		}
		info, err := os.Lstat(absolute)
		if err != nil { return nil, fmt.Errorf("inspect scan path %q: %w", clean, err) }
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("scan path %q must be a regular non-symlink file", clean)
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil { return nil, fmt.Errorf("make scan path repository-relative: %w", err) }
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("scan path %q escapes repository root", clean)
		}
		relative := filepath.ToSlash(rel)
		if !eligible[relative] { return nil, fmt.Errorf("scan path %q is not in the eligible analyzer source inventory", relative) }
		if seen[relative] { return nil, fmt.Errorf("duplicate scan path %q", relative) }
		seen[relative] = true
		out = append(out, relative)
	}
	sort.Strings(out)
	return out, nil
}
