package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CheckovScanner runs a version-pinned Checkov CLI and normalizes SARIF 2.1.0.
// ConfigPath must be repository-relative (typically rules/checkov.yml).
type CheckovScanner struct {
	Binary                 string
	BinarySHA256           string
	Version                string
	ConfigPath             string
	RuleSetSHA256          string
	Frameworks             []string
	ScanPaths              []string
	PrecomputedInputSHA256 string
	Timeout                time.Duration
	Runner                 CommandRunner
}

func (s CheckovScanner) Scan(ctx context.Context, root string) (ScanResult, error) {
	out := ScanResult{
		SchemaVersion: "1",
		SARIFVersion:  SARIFVersion,
		Status:        ScanStatusError,
		Analyzers:     []AnalyzerInfo{{Name: "checkov", Version: strings.TrimSpace(s.Version)}},
		Findings:      []Finding{},
		Errors:        []string{},
	}
	if strings.TrimSpace(s.Version) == "" {
		return out, fmt.Errorf("Checkov version must be explicitly pinned")
	}
	if strings.TrimSpace(s.ConfigPath) == "" {
		return out, fmt.Errorf("Checkov ConfigPath is required")
	}
	binary := strings.TrimSpace(s.Binary)
	if binary == "" {
		binary = "checkov"
	}
	if root == "" {
		root = "."
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return out, fmt.Errorf("resolve checkov root: %w", err)
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return out, fmt.Errorf("resolve checkov root symlinks: %w", err)
	}
	rootInfo, err := os.Stat(rootAbs)
	if err != nil {
		return out, fmt.Errorf("inspect checkov root: %w", err)
	}
	if !rootInfo.IsDir() {
		return out, fmt.Errorf("checkov root %q is not a directory", rootAbs)
	}

	binarySHA256 := ""
	binaryDigestVerified := false
	if strings.TrimSpace(s.BinarySHA256) != "" {
		expected := strings.ToLower(strings.TrimSpace(s.BinarySHA256))
		if !validPlanSHA256(expected) {
			return out, fmt.Errorf("expected Checkov binary SHA-256 is invalid")
		}
		resolvedBinary, resolveErr := ResolveExecutable(binary)
		if resolveErr != nil {
			return out, resolveErr
		}
		actual, hashErr := HashFileCached(resolvedBinary)
		if hashErr != nil {
			return out, fmt.Errorf("hash Checkov executable: %w", hashErr)
		}
		if actual != expected {
			return out, fmt.Errorf("Checkov binary SHA-256 mismatch: expected %s but found %s", expected, actual)
		}
		binary, binarySHA256 = resolvedBinary, actual
		binaryDigestVerified = true
		out.BinarySHA256 = actual
	}

	configAbs, configRel, err := resolveConfig(rootAbs, s.ConfigPath)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	configHash, err := HashFile(configAbs)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	ruleSetHash := strings.ToLower(strings.TrimSpace(s.RuleSetSHA256))
	if ruleSetHash == "" {
		ruleSetHash = configHash
	}
	if !validPlanSHA256(ruleSetHash) {
		return out, fmt.Errorf("Checkov rule-set SHA-256 is invalid")
	}
	out.ConfigurationPath = configRel
	out.ConfigurationSHA256 = configHash
	out.RuleSetSHA256 = ruleSetHash

	inputHash := strings.ToLower(strings.TrimSpace(s.PrecomputedInputSHA256))
	if inputHash != "" {
		if !validPlanSHA256(inputHash) {
			return out, fmt.Errorf("precomputed Checkov input SHA-256 is invalid")
		}
	} else {
		inputHash, err = hashInputTree(rootAbs)
		if err != nil {
			out.Errors = append(out.Errors, err.Error())
			return out, err
		}
	}
	out.InputSHA256 = inputHash

	selectedPaths, err := normalizeScanPaths(rootAbs, s.ScanPaths, configRel)
	if err != nil {
		return out, fmt.Errorf("validate requested checkov scan paths: %w", err)
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tmp, err := os.MkdirTemp("", "pathfinder-checkov-*")
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	defer os.RemoveAll(tmp)
	home := filepath.Join(tmp, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}

	runner := s.Runner
	if runner == nil {
		runner = ExecCommandRunner{HomeDir: home}
	}

	if !binaryDigestVerified {
		versionResult, err := runner.Run(runCtx, binary, []string{"--version"}, rootAbs)
		if err != nil {
			out.Errors = append(out.Errors, err.Error())
			return out, err
		}
		if versionResult.ExitCode != 0 {
			msg := fmt.Sprintf("Checkov version probe exited %d: %s", versionResult.ExitCode, truncate(string(versionResult.Stderr), 4096))
			out.Errors = append(out.Errors, msg)
			return out, fmt.Errorf("%s", msg)
		}
		versionOutput := strings.TrimSpace(string(versionResult.Stdout) + " " + string(versionResult.Stderr))
		if !strings.Contains(versionOutput, s.Version) {
			err := fmt.Errorf("Checkov version mismatch: expected %q, command reported %q", s.Version, truncate(versionOutput, 512))
			out.Errors = append(out.Errors, err.Error())
			return out, err
		}
	}

	frameworks := s.Frameworks
	if len(frameworks) == 0 {
		frameworks = []string{"terraform", "cloudformation", "kubernetes", "dockerfile"}
	}
	args := []string{
		"-d", rootAbs,
		"--config-file", configAbs,
		"--output", "sarif",
		"--output-file-path", tmp + string(filepath.Separator),
		"--download-external-modules", "false",
		"--quiet",
	}
	for _, fw := range frameworks {
		args = append(args, "--framework", fw)
	}
	if len(selectedPaths) > 0 {
		// Checkov -d scans a directory; scoped inventories are recorded for
		// evidence even when the CLI still walks the declared root.
		_ = selectedPaths
	}

	result, runErr := runner.Run(runCtx, binary, args, rootAbs)
	if runErr != nil {
		out.Errors = append(out.Errors, runErr.Error())
		return out, runErr
	}
	// Checkov exits 1 when findings exist; treat only launch failures as errors.
	if result.ExitCode != 0 && result.ExitCode != 1 {
		msg := fmt.Sprintf("checkov exited %d: %s", result.ExitCode, truncate(string(result.Stderr), 4096))
		out.Errors = append(out.Errors, msg)
		return out, fmt.Errorf("%s", msg)
	}

	sarifPath, err := findCheckovSARIF(tmp)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}
	raw, err := os.ReadFile(sarifPath)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, err
	}

	parsed, err := ParseSARIF(raw, rootAbs)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, fmt.Errorf("normalize Checkov SARIF: %w", err)
	}
	if len(parsed.Analyzers) == 0 || !strings.Contains(strings.ToLower(parsed.Analyzers[0].Name), "checkov") {
		return out, fmt.Errorf("Checkov returned unexpected SARIF tool metadata: %+v", parsed.Analyzers)
	}
	for i := range parsed.Analyzers {
		parsed.Analyzers[i].Name = "checkov"
		if parsed.Analyzers[i].Version == "" {
			parsed.Analyzers[i].Version = s.Version
		}
	}
	for i := range parsed.Findings {
		parsed.Findings[i].Analyzer = "checkov"
		if parsed.Findings[i].AnalyzerVersion == "" {
			parsed.Findings[i].AnalyzerVersion = s.Version
		}
		parsed.Findings[i].RuleID = normalizeCheckovRuleID(parsed.Findings[i].RuleID)
	}
	parsed.ConfigurationPath = configRel
	parsed.ConfigurationSHA256 = configHash
	parsed.BinarySHA256 = binarySHA256
	parsed.RuleSetSHA256 = ruleSetHash
	parsed.InputSHA256 = inputHash
	if len(selectedPaths) > 0 {
		parsed.ScannedPaths = append([]string(nil), selectedPaths...)
	} else {
		parsed.ScannedPaths, err = discoverScannedPathsExcluding(rootAbs, configRel)
		if err != nil {
			return out, fmt.Errorf("discover checkov scanner scope: %w", err)
		}
	}
	if parsed.Status != ScanStatusComplete {
		return parsed, fmt.Errorf("Checkov scan was incomplete; refusing to treat partial evidence as complete")
	}
	return parsed, nil
}

func findCheckovSARIF(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "results_sarif.sarif"),
		filepath.Join(dir, "results.sarif"),
		filepath.Join(dir, "checkov.sarif"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.Mode().IsRegular() {
			return c, nil
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read checkov output dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			nested, nestErr := findCheckovSARIF(filepath.Join(dir, e.Name()))
			if nestErr == nil {
				return nested, nil
			}
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".sarif") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("checkov SARIF output not found in %s", dir)
}

func normalizeCheckovRuleID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.LastIndex(id, "/"); i >= 0 && i+1 < len(id) {
		id = id[i+1:]
	}
	if i := strings.LastIndex(id, "."); i >= 0 && i+1 < len(id) {
		tail := id[i+1:]
		upper := strings.ToUpper(tail)
		if strings.HasPrefix(upper, "CKV_") || strings.HasPrefix(upper, "CKV2_") {
			id = tail
		}
	}
	return id
}
