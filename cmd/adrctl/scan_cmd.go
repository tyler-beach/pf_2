package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/orchestrate"
	"github.com/tyler-beach/pf_2/internal/plan"
)

func runOpenGrepScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("scan-opengrep", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root to scan")
	config := fs.String("config", "rules/pathfinder-security.yml", "repository-relative OpenGrep rule file")
	scope := fs.String("scope", ".", "comma-separated repository-relative scope prefixes (narrow for PR diffs)")
	binary := fs.String("binary", "opengrep", "OpenGrep executable path")
	binarySHA256 := fs.String("binary-sha256", "", "expected OpenGrep executable SHA-256 (computed and recorded if omitted)")
	version := fs.String("version", "1.30.0", "expected pinned OpenGrep version")
	timeout := fs.Duration("timeout", 5*time.Minute, "scan timeout (for example 90s or 5m)")
	jsonPath := fs.String("json-out", "analyzer-evidence/opengrep.json", "normalized analyzer evidence output")
	sarifPath := fs.String("sarif-out", "analyzer-evidence/opengrep.sarif", "raw SARIF output for audit and troubleshooting")
	manifestPath := fs.String("manifest-out", "analyzer-evidence/run-manifest.json", "deterministic run manifest output")
	cacheDir := fs.String("cache-dir", defaultAnalyzerCacheDirectory(), "optional persistent cache directory (empty disables caching)")
	maxConcurrency := fs.Int("max-concurrency", 1, "maximum concurrent analyzer tasks")
	maxAttempts := fs.Int("max-attempts", 2, "attempts per task, including the first")
	maxMemoryMB := fs.Int("max-memory-mb", 1024, "worker resource-profile memory budget recorded in the plan")
	maxFileBytes := fs.Int64("max-file-bytes", 4<<20, "maximum supported source file size")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("scan-opengrep accepts no positional arguments")
	}
	scopePrefixes := splitScopePrefixes(*scope)
	if len(scopePrefixes) == 0 {
		return fmt.Errorf("--scope must declare at least one repository-relative prefix")
	}
	if *timeout <= 0 {
		return fmt.Errorf("--timeout must be positive")
	}
	if *maxConcurrency < 1 || *maxConcurrency > plan.MaxConcurrency {
		return fmt.Errorf("--max-concurrency must be between 1 and %d", plan.MaxConcurrency)
	}
	if *maxAttempts < 1 || *maxAttempts > plan.MaxRetryCount+1 {
		return fmt.Errorf("--max-attempts must be between 1 and %d", plan.MaxRetryCount+1)
	}
	if *maxMemoryMB < 1 {
		return fmt.Errorf("--max-memory-mb must be positive")
	}
	if *maxFileBytes < 1 {
		return fmt.Errorf("--max-file-bytes must be positive")
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	binaryPath, binaryHash, err := resolveOpenGrepBinary(*binary, *binarySHA256)
	if err != nil {
		return err
	}
	_, configRel, err := resolveRepositoryConfig(rootAbs, *config)
	if err != nil {
		return err
	}
	maxRetries := *maxAttempts - 1
	spec := plan.AnalyzerSpec{
		Name: "opengrep", Version: *version, BinarySHA256: binaryHash, RuleSetSHA256: "",
		ConfigurationPath: configRel, ScopePrefixes: scopePrefixes,
		TimeoutMillis: timeout.Milliseconds(), MaxRetries: maxRetries,
		MaxOutputBytes: 64 << 20, RetryBackoffMillis: 500, MaxRetryBackoffMillis: 10_000,
		RetryableFailureKinds: []string{"timeout", "temporary_process_start"},
	}
	_ = maxFileBytes // retained for CLI compatibility; plan inventory enforces file size limits
	_ = maxMemoryMB  // retained as documented plan metadata; local OS limits are not enforced
	manifest, err := plan.BuildManifest(rootAbs, []plan.AnalyzerSpec{spec}, *maxConcurrency)
	if err != nil {
		return fmt.Errorf("build deterministic analyzer plan: %w", err)
	}
	var cache plan.ResultCache
	if strings.TrimSpace(*cacheDir) != "" {
		fileCache, cacheErr := plan.NewFileCache(*cacheDir)
		if cacheErr != nil {
			return fmt.Errorf("open analyzer cache: %w", cacheErr)
		}
		cache = fileCache
	}
	run, err := orchestrate.DefaultPlanExecutor{Observer: cliObserver()}.Execute(ctx, rootAbs, manifest, plan.OpenGrepTaskRunner{Binary: binaryPath}, cache)
	if err != nil && run.Status == "" {
		return fmt.Errorf("run deterministic analyzer plan: %w", err)
	}
	if len(run.Tasks) != 1 {
		return fmt.Errorf("OpenGrep scan plan returned %d tasks; expected exactly one", len(run.Tasks))
	}
	task := run.Tasks[0]
	var result analyzer.ScanResult
	if task.Evidence != nil {
		result = *task.Evidence
		result.RawSARIF = append([]byte(nil), task.RawSARIF...)
	}
	compat := scanRunManifest{
		SchemaVersion:  "1",
		ManifestID:     run.PlanManifestSHA256,
		SnapshotSHA256: run.InputSHA256,
		PlanSHA256:     run.PlanManifestSHA256,
		Status:         run.Status,
		Tasks: []scanRunTask{{
			TaskID: task.TaskID, CacheKey: task.CacheKey, Analyzer: task.Analyzer,
			Status: task.Status, Attempts: task.Attempts, Cached: task.CacheHit,
			Result: result, Error: task.Error,
		}},
	}
	if err != nil {
		compat.Errors = []string{err.Error()}
	}
	manifestBytes, encodeErr := json.MarshalIndent(compat, "", "  ")
	if encodeErr != nil {
		return fmt.Errorf("encode run manifest: %w", encodeErr)
	}
	if writeErr := writeFile(*manifestPath, append(manifestBytes, '\n')); writeErr != nil {
		return writeErr
	}
	data, encodeErr := analyzer.WriteJSON(result)
	if encodeErr != nil {
		return encodeErr
	}
	if err := writeFile(*jsonPath, data); err != nil {
		return err
	}
	if len(result.RawSARIF) > 0 {
		if err := writeFile(*sarifPath, result.RawSARIF); err != nil {
			return err
		}
	}
	if run.Status != plan.RunStatusComplete || task.Status != plan.TaskStatusComplete {
		reason := task.Error
		if reason == "" {
			reason = "one or more planned scan tasks did not complete"
		}
		return fmt.Errorf("OpenGrep assessment %s; manifest: %s; normalized evidence: %s; raw SARIF: %s: %s",
			run.Status, *manifestPath, *jsonPath, *sarifPath, reason)
	}
	fmt.Printf("OpenGrep %s: %d finding(s); cache hit: %t; manifest: %s; evidence: %s; SARIF: %s; configuration SHA-256: %s; rule-set SHA-256: %s; input SHA-256: %s\n",
		*version, len(result.Findings), task.CacheHit, *manifestPath, *jsonPath, *sarifPath,
		result.ConfigurationSHA256, result.RuleSetSHA256, result.InputSHA256)
	return nil
}

func runCheckovScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("scan-checkov", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root to scan")
	config := fs.String("config", "rules/checkov.yml", "repository-relative Checkov configuration file")
	scope := fs.String("scope", ".", "comma-separated repository-relative scope prefixes (narrow for PR diffs)")
	binary := fs.String("binary", "checkov", "Checkov executable path")
	binarySHA256 := fs.String("binary-sha256", "", "expected Checkov executable SHA-256 (computed and recorded if omitted)")
	version := fs.String("version", "3.2.484", "expected pinned Checkov version")
	timeout := fs.Duration("timeout", 5*time.Minute, "scan timeout (for example 90s or 5m)")
	jsonPath := fs.String("json-out", "analyzer-evidence/checkov.json", "normalized analyzer evidence output")
	sarifPath := fs.String("sarif-out", "analyzer-evidence/checkov.sarif", "raw SARIF output for audit and troubleshooting")
	manifestPath := fs.String("manifest-out", "analyzer-evidence/checkov-run-manifest.json", "deterministic run manifest output")
	cacheDir := fs.String("cache-dir", defaultAnalyzerCacheDirectory(), "optional persistent cache directory (empty disables caching)")
	maxConcurrency := fs.Int("max-concurrency", 1, "maximum concurrent analyzer tasks")
	maxAttempts := fs.Int("max-attempts", 2, "attempts per task, including the first")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("scan-checkov accepts no positional arguments")
	}
	scopePrefixes := splitScopePrefixes(*scope)
	if len(scopePrefixes) == 0 {
		return fmt.Errorf("--scope must declare at least one repository-relative prefix")
	}
	if *timeout <= 0 {
		return fmt.Errorf("--timeout must be positive")
	}
	if *maxConcurrency < 1 || *maxConcurrency > plan.MaxConcurrency {
		return fmt.Errorf("--max-concurrency must be between 1 and %d", plan.MaxConcurrency)
	}
	if *maxAttempts < 1 || *maxAttempts > plan.MaxRetryCount+1 {
		return fmt.Errorf("--max-attempts must be between 1 and %d", plan.MaxRetryCount+1)
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	binaryPath, binaryHash, err := resolvePinnedBinary(*binary, *binarySHA256, "Checkov")
	if err != nil {
		return err
	}
	_, configRel, err := resolveRepositoryConfig(rootAbs, *config)
	if err != nil {
		return err
	}
	maxRetries := *maxAttempts - 1
	spec := plan.AnalyzerSpec{
		Name: "checkov", Version: *version, BinarySHA256: binaryHash, RuleSetSHA256: "",
		ConfigurationPath: configRel, ScopePrefixes: scopePrefixes,
		TimeoutMillis: timeout.Milliseconds(), MaxRetries: maxRetries,
		MaxOutputBytes: 64 << 20, RetryBackoffMillis: 500, MaxRetryBackoffMillis: 10_000,
		RetryableFailureKinds: []string{"timeout", "temporary_process_start"},
	}
	manifest, err := plan.BuildManifest(rootAbs, []plan.AnalyzerSpec{spec}, *maxConcurrency)
	if err != nil {
		return fmt.Errorf("build deterministic analyzer plan: %w", err)
	}
	var cache plan.ResultCache
	if strings.TrimSpace(*cacheDir) != "" {
		fileCache, cacheErr := plan.NewFileCache(*cacheDir)
		if cacheErr != nil {
			return fmt.Errorf("open analyzer cache: %w", cacheErr)
		}
		cache = fileCache
	}
	run, err := orchestrate.DefaultPlanExecutor{Observer: cliObserver()}.Execute(ctx, rootAbs, manifest, plan.CheckovTaskRunner{Binary: binaryPath}, cache)
	if err != nil && run.Status == "" {
		return fmt.Errorf("run deterministic analyzer plan: %w", err)
	}
	if len(run.Tasks) != 1 {
		return fmt.Errorf("Checkov scan plan returned %d tasks; expected exactly one", len(run.Tasks))
	}
	task := run.Tasks[0]
	var result analyzer.ScanResult
	if task.Evidence != nil {
		result = *task.Evidence
		result.RawSARIF = append([]byte(nil), task.RawSARIF...)
	}
	compat := scanRunManifest{
		SchemaVersion:  "1",
		ManifestID:     run.PlanManifestSHA256,
		SnapshotSHA256: run.InputSHA256,
		PlanSHA256:     run.PlanManifestSHA256,
		Status:         run.Status,
		Tasks: []scanRunTask{{
			TaskID: task.TaskID, CacheKey: task.CacheKey, Analyzer: task.Analyzer,
			Status: task.Status, Attempts: task.Attempts, Cached: task.CacheHit,
			Result: result, Error: task.Error,
		}},
	}
	if err != nil {
		compat.Errors = []string{err.Error()}
	}
	manifestBytes, encodeErr := json.MarshalIndent(compat, "", "  ")
	if encodeErr != nil {
		return fmt.Errorf("encode run manifest: %w", encodeErr)
	}
	if writeErr := writeFile(*manifestPath, append(manifestBytes, '\n')); writeErr != nil {
		return writeErr
	}
	data, encodeErr := analyzer.WriteJSON(result)
	if encodeErr != nil {
		return encodeErr
	}
	if err := writeFile(*jsonPath, data); err != nil {
		return err
	}
	if len(result.RawSARIF) > 0 {
		if err := writeFile(*sarifPath, result.RawSARIF); err != nil {
			return err
		}
	}
	if run.Status != plan.RunStatusComplete || task.Status != plan.TaskStatusComplete {
		reason := task.Error
		if reason == "" {
			reason = "one or more planned scan tasks did not complete"
		}
		return fmt.Errorf("Checkov assessment %s; manifest: %s; normalized evidence: %s; raw SARIF: %s: %s",
			run.Status, *manifestPath, *jsonPath, *sarifPath, reason)
	}
	fmt.Printf("Checkov %s: %d finding(s); cache hit: %t; manifest: %s; evidence: %s; SARIF: %s; configuration SHA-256: %s; rule-set SHA-256: %s; input SHA-256: %s\n",
		*version, len(result.Findings), task.CacheHit, *manifestPath, *jsonPath, *sarifPath,
		result.ConfigurationSHA256, result.RuleSetSHA256, result.InputSHA256)
	return nil
}

// scanRunManifest is the scan-opengrep compatibility envelope over plan.RunResult.
// It preserves CI fields (manifest_id, cached) while planning/execution use internal/plan.
type scanRunManifest struct {
	SchemaVersion  string        `json:"schema_version"`
	ManifestID     string        `json:"manifest_id"`
	SnapshotSHA256 string        `json:"snapshot_sha256"`
	PlanSHA256     string        `json:"plan_sha256"`
	Status         string        `json:"status"`
	Tasks          []scanRunTask `json:"tasks"`
	Errors         []string      `json:"errors,omitempty"`
}

type scanRunTask struct {
	TaskID   string              `json:"task_id"`
	CacheKey string              `json:"cache_key"`
	Analyzer string              `json:"analyzer"`
	Status   string              `json:"status"`
	Attempts int                 `json:"attempts"`
	Cached   bool                `json:"cached"`
	Result   analyzer.ScanResult `json:"result"`
	Error    string              `json:"error,omitempty"`
}
