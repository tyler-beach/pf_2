package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func loadVerifiedAnalyzerEvidence(root string, repoMode bool, evidenceCSV, sarifCSV string) ([]analyzer.ScanResult, error) {
	evidencePaths := splitCSV(evidenceCSV)
	sarifPaths := splitCSV(sarifCSV)
	if len(evidencePaths) == 0 && len(sarifPaths) == 0 {
		return nil, nil
	}
	if !repoMode {
		return nil, fmt.Errorf("--analyzer-evidence is supported only for repository assessments")
	}
	if len(evidencePaths) > 0 && len(sarifPaths) == 0 {
		return nil, fmt.Errorf("--analyzer-sarif is required when --analyzer-evidence is provided")
	}
	if len(sarifPaths) > 0 && len(evidencePaths) == 0 {
		return nil, fmt.Errorf("--analyzer-sarif requires --analyzer-evidence")
	}
	if len(evidencePaths) != len(sarifPaths) {
		return nil, fmt.Errorf("--analyzer-evidence and --analyzer-sarif must list the same number of paths (got %d evidence, %d sarif)", len(evidencePaths), len(sarifPaths))
	}
	out := make([]analyzer.ScanResult, 0, len(evidencePaths))
	for i := range evidencePaths {
		data, err := os.ReadFile(evidencePaths[i])
		if err != nil {
			return nil, fmt.Errorf("read analyzer evidence %q: %w", evidencePaths[i], err)
		}
		var evidence analyzer.ScanResult
		if err := json.Unmarshal(data, &evidence); err != nil {
			return nil, fmt.Errorf("decode analyzer evidence %q: %w", evidencePaths[i], err)
		}
		rawSARIF, err := os.ReadFile(sarifPaths[i])
		if err != nil {
			return nil, fmt.Errorf("read raw analyzer SARIF %q: %w", sarifPaths[i], err)
		}
		verified, err := analyzer.VerifyScanEvidence(root, evidence, rawSARIF)
		if err != nil {
			return nil, fmt.Errorf("verify analyzer evidence %q: %w", evidencePaths[i], err)
		}
		out = append(out, verified)
	}
	return out, nil
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitScopePrefixes(value string) []string {
	return splitCSV(value)
}

// resolveOpenGrepBinary resolves the exact executable file and records its digest.
// If an expected digest is supplied, a mismatch fails before the analyzer starts.
func resolveOpenGrepBinary(binary, expectedSHA256 string) (string, string, error) {
	return resolvePinnedBinary(binary, expectedSHA256, "OpenGrep")
}

func resolvePinnedBinary(binary, expectedSHA256, label string) (string, string, error) {
	resolved, err := analyzer.ResolveExecutable(binary)
	if err != nil {
		return "", "", fmt.Errorf("resolve %s executable: %w", label, err)
	}
	actual, err := analyzer.HashFileCached(resolved)
	if err != nil {
		return "", "", fmt.Errorf("hash %s executable: %w", label, err)
	}
	expected := strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expected != "" {
		if len(expected) != 64 {
			return "", "", fmt.Errorf("expected binary SHA-256 must be a 64-character hexadecimal digest")
		}
		if _, err := hex.DecodeString(expected); err != nil {
			return "", "", fmt.Errorf("expected binary SHA-256 is invalid: %w", err)
		}
		if actual != expected {
			return "", "", fmt.Errorf("%s binary SHA-256 mismatch: expected %s but found %s", label, expected, actual)
		}
	}
	return resolved, actual, nil
}

func defaultAnalyzerCacheDirectory() string {
	root, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, "pathfinder", "analyzer-cache")
}

func resolveRepositoryConfig(root, requested string) (string, string, error) {
	requested = strings.TrimSpace(strings.ReplaceAll(requested, "\\", "/"))
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(requested)))
	if clean == "" || clean == "." || filepath.IsAbs(filepath.FromSlash(clean)) ||
		(len(clean) >= 2 && clean[1] == ':') || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "", fmt.Errorf("OpenGrep configuration path must be a repository-relative file")
	}
	candidate := filepath.Join(root, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", "", fmt.Errorf("resolve OpenGrep configuration: %w", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", "", fmt.Errorf("make configuration path repository-relative: %w", err)
	}
	relSlash := filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("OpenGrep configuration path escapes repository root")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("inspect OpenGrep configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("OpenGrep configuration must be a regular file")
	}
	return resolved, relSlash, nil
}

func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory for %s: %w", path, err)
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// canonicalPathForCollision resolves existing symlinks and canonicalizes paths whose
// final file does not exist yet. This prevents analyzer inputs from aliasing outputs
// through relative paths or an existing symlink.
func canonicalPathForCollision(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	abs = filepath.Clean(abs)
	current := abs
	suffix := make([]string, 0, 4)
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		if !os.IsNotExist(resolveErr) {
			return abs
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
