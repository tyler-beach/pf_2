package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/orchestrate"
	"github.com/tyler-beach/pf_2/internal/plan"
)

// runPlanRepo creates a deterministic, source-bound execution plan. Planning
// does not launch analyzers; every execution must re-verify the manifest before
// using its cache keys or dispatching tasks.
func runPlanRepo(args []string) error {
	fs := flag.NewFlagSet("plan-repo", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	analyzerName := fs.String("analyzer", "opengrep", "analyzer adapter name")
	version := fs.String("version", "1.33.0", "exact analyzer version")
	config := fs.String("config", "rules/pathfinder-security.yml", "repository-relative analyzer configuration")
	binarySHA256 := fs.String("binary-sha256", "", "pinned executable SHA-256; required for OpenGrep execution")
	ruleSetSHA256 := fs.String("ruleset-sha256", "", "optional rule-set SHA-256 (defaults to configuration SHA-256)")
	scope := fs.String("scope", ".", "comma-separated repository-relative scope prefixes")
	timeout := fs.Duration("timeout", 5*time.Minute, "per-attempt timeout (for example 90s or 5m)")
	maxRetries := fs.Int("max-retries", 1, "maximum retries after a retryable failure (0-5)")
	maxConcurrency := fs.Int("max-concurrency", 2, "bounded parallel task limit (1-16)")
	maxOutputBytes := fs.Int64("max-output-bytes", 64<<20, "maximum raw analyzer output bytes")
	backoff := fs.Duration("retry-backoff", 500*time.Millisecond, "initial retry backoff")
	maxBackoff := fs.Duration("max-retry-backoff", 10*time.Second, "maximum retry backoff")
	output := fs.String("json-out", "analyzer-evidence/plan.json", "deterministic plan manifest output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("plan-repo accepts no positional arguments")
	}
	if *timeout < time.Millisecond || *backoff < 0 || *maxBackoff < 0 {
		return fmt.Errorf("timeouts and retry backoffs must not be negative, and timeout must be at least 1ms")
	}
	scopePrefixes := []string{}
	for _, item := range strings.Split(*scope, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			scopePrefixes = append(scopePrefixes, item)
		}
	}
	spec := plan.AnalyzerSpec{
		Name: *analyzerName, Version: *version, BinarySHA256: *binarySHA256, RuleSetSHA256: *ruleSetSHA256,
		ConfigurationPath: *config, ScopePrefixes: scopePrefixes, TimeoutMillis: timeout.Milliseconds(), MaxRetries: *maxRetries,
		MaxOutputBytes: *maxOutputBytes, RetryBackoffMillis: backoff.Milliseconds(),
		MaxRetryBackoffMillis: maxBackoff.Milliseconds(),
		RetryableFailureKinds: []string{"timeout", "temporary_process_start"},
	}
	manifest, err := plan.BuildManifest(*root, []plan.AnalyzerSpec{spec}, *maxConcurrency)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode analyzer plan manifest: %w", err)
	}
	data = append(data, '\n')
	if err := writeFile(*output, data); err != nil {
		return err
	}
	fmt.Printf("Plan %s; input %s; tasks %d; concurrency %d; output %s. No analyzer was executed.\n",
		manifest.ManifestSHA256, manifest.InputSHA256, len(manifest.Tasks), manifest.MaxConcurrency, *output)
	return nil
}

func readPlanManifest(path string) (plan.Manifest, error) {
	var manifest plan.Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("read plan manifest: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode plan manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return manifest, fmt.Errorf("decode plan manifest: trailing JSON value")
		}
		return manifest, fmt.Errorf("decode plan manifest: trailing content: %w", err)
	}
	return manifest, nil
}

func runVerifyPlan(args []string) error {
	fs := flag.NewFlagSet("verify-plan", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	manifestPath := fs.String("manifest", "analyzer-evidence/plan.json", "plan manifest JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("verify-plan accepts no positional arguments")
	}
	manifest, err := readPlanManifest(*manifestPath)
	if err != nil {
		return err
	}
	if err := plan.VerifyManifest(*root, manifest); err != nil {
		return err
	}
	fmt.Printf("Plan verified: %s; input %s; tasks %d.\n", manifest.ManifestSHA256, manifest.InputSHA256, len(manifest.Tasks))
	return nil
}

func runExecutePlan(ctx context.Context, args []string) error {
	return runExecutePlanWithRunner(ctx, args, nil)
}

// runExecutePlanWithRunner supports deterministic CLI integration tests while
// keeping production runner selection explicit and pinned by the plan manifest.
func runExecutePlanWithRunner(ctx context.Context, args []string, injected plan.TaskRunner) error {
	fs := flag.NewFlagSet("execute-plan", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	manifestPath := fs.String("manifest", "analyzer-evidence/plan.json", "source-bound plan manifest")
	binary := fs.String("binary", "opengrep", "OpenGrep executable path")
	checkovBinary := fs.String("checkov-binary", "checkov", "Checkov executable path")
	cacheDirDefault, cacheErr := os.UserCacheDir()
	if cacheErr != nil {
		cacheDirDefault = os.TempDir()
	}
	cacheDir := fs.String("cache-dir", filepath.Join(cacheDirDefault, "pathfinder", "analyzers"), "private local cache directory (prefer outside repository root)")
	output := fs.String("json-out", "analyzer-evidence/execution.json", "execution result manifest JSON")
	sarifDir := fs.String("sarif-dir", "analyzer-evidence/raw", "directory for per-task raw SARIF artifacts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("execute-plan accepts no positional arguments")
	}
	manifest, err := readPlanManifest(*manifestPath)
	if err != nil {
		return err
	}
	if err := plan.VerifyManifest(*root, manifest); err != nil {
		return fmt.Errorf("verify plan before execution: %w", err)
	}
	cache, err := plan.NewFileCache(*cacheDir)
	if err != nil {
		return err
	}
	runner := injected
	if runner == nil {
		runner = plan.DefaultRunnerRegistryWithBinaries(*binary, *checkovBinary)
	}
	executor := orchestrate.DefaultPlanExecutor{Observer: cliObserver()}
	result, runErr := executor.Execute(ctx, *root, manifest, runner, cache)
	for i := range result.Tasks {
		task := &result.Tasks[i]
		if len(task.RawSARIF) == 0 {
			continue
		}
		path := filepath.Join(*sarifDir, task.TaskID+".sarif")
		if err := writeFile(path, task.RawSARIF); err != nil {
			task.Status = plan.TaskStatusError
			task.Error = "write raw SARIF artifact: " + err.Error()
			result.Status = plan.RunStatusPartial
			if runErr == nil {
				runErr = err
			}
			continue
		}
		task.RawSARIFPath = path
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode execution result manifest: %w", err)
	}
	if err := writeFile(*output, append(data, '\n')); err != nil {
		return err
	}
	fmt.Printf("Execution %s; plan %s; tasks %d; output %s.\n", result.Status, result.PlanManifestSHA256, len(result.Tasks), *output)
	if runErr != nil {
		return runErr
	}
	if result.Status != plan.RunStatusComplete {
		return fmt.Errorf("execution did not complete successfully; status %s; inspect %s", result.Status, *output)
	}
	return nil
}
