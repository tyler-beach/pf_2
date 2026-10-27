package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
	"github.com/tyler-beach/pf_2/internal/plan"
	"github.com/tyler-beach/pf_2/internal/workshop"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "validate":
		err = runValidate(os.Args[2:])
	case "assess":
		err = runAssess(os.Args[2:], false)
	case "assess-repo":
		err = runAssess(os.Args[2:], true)
	case "ingest-adrs":
		err = runIngestADRs(os.Args[2:])
	case "extract-obligations":
		err = runExtractObligations(os.Args[2:])
	case "assist-obligations":
		err = runAssistObligations(os.Args[2:])
	case "scan-opengrep":
		err = runOpenGrepScan(os.Args[2:])
	case "plan-repo":
		err = runPlanRepo(os.Args[2:])
	case "verify-plan":
		err = runVerifyPlan(os.Args[2:])
	case "execute-plan":
		err = runExecutePlan(os.Args[2:])
	case "assert-issue-verdict":
		err = runAssertIssueVerdict(os.Args[2:])
	case "review-init":
		err = runInitReviewRegistry(os.Args[2:])
	case "record-obligation-review":
		err = runRecordObligationReview(os.Args[2:])
	case "validate-obligation-reviews":
		err = runValidateObligationReviews(os.Args[2:])
	case "assess-obligations":
		err = runAssessObligations(os.Args[2:])
	case "workshop-emit":
		err = runWorkshopEmit(os.Args[2:])
	case "specification-check":
		err = runSpecificationCheck(os.Args[2:])
	case "version":
		fmt.Println("adrctl 0.1.0")
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "adrctl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: adrctl <validate|assess|assess-repo|ingest-adrs|extract-obligations|assist-obligations|scan-opengrep|plan-repo|verify-plan|execute-plan|assert-issue-verdict|review-init|record-obligation-review|validate-obligation-reviews|assess-obligations|workshop-emit|specification-check|version> [flags]")
}

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	path := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := assessment.LoadCatalog(*path)
	if err != nil {
		return err
	}
	n := 0
	for _, a := range c.ADRs {
		n += len(a.Requirements)
	}
	fmt.Printf("Catalog valid: %d ADR(s), %d requirement(s).\n", len(c.ADRs), n)
	return nil
}

func runAssess(args []string, repoMode bool) error {
	fs := flag.NewFlagSet("assess", flag.ContinueOnError)
	input := fs.String("input", "", "Markdown issue body")
	root := fs.String("root", ".", "repository root")
	catalogPath := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	reportPath := fs.String("report", "report.md", "Markdown report output")
	jsonPath := fs.String("json-out", "assessment.json", "JSON evidence output")
	sarifPath := fs.String("sarif-out", "results.sarif", "SARIF output")
	analyzerEvidencePath := fs.String("analyzer-evidence", "", "normalized analyzer scan evidence JSON (repository assessment only)")
	analyzerSARIFPath := fs.String("analyzer-sarif", "", "raw SARIF paired with normalized analyzer evidence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	var files []assessment.SourceFile
	if repoMode {
		files, err = assessment.LoadSourceFiles(*root)
	} else {
		if *input == "" {
			return fmt.Errorf("--input is required for issue assessment")
		}
		var data []byte
		data, err = os.ReadFile(*input)
		if err == nil {
			files = assessment.ParseIssueMarkdown(string(data))
		}
	}
	if err != nil {
		return err
	}
	mode := assessment.EvidenceModeSubmission
	if repoMode {
		mode = assessment.EvidenceModeRepository
	}
	var analyzerResults []analyzer.ScanResult
	if *analyzerEvidencePath != "" {
		if !repoMode {
			return fmt.Errorf("--analyzer-evidence is supported only for repository assessments")
		}
		if *analyzerSARIFPath == "" {
			return fmt.Errorf("--analyzer-sarif is required when --analyzer-evidence is provided")
		}
		data, readErr := os.ReadFile(*analyzerEvidencePath)
		if readErr != nil {
			return fmt.Errorf("read analyzer evidence: %w", readErr)
		}
		var evidence analyzer.ScanResult
		if decodeErr := json.Unmarshal(data, &evidence); decodeErr != nil {
			return fmt.Errorf("decode analyzer evidence: %w", decodeErr)
		}
		rawSARIF, readErr := os.ReadFile(*analyzerSARIFPath)
		if readErr != nil {
			return fmt.Errorf("read raw analyzer SARIF: %w", readErr)
		}
		verified, verifyErr := analyzer.VerifyScanEvidence(*root, evidence, rawSARIF)
		if verifyErr != nil {
			return fmt.Errorf("verify analyzer evidence: %w", verifyErr)
		}
		analyzerResults = append(analyzerResults, verified)
	} else if *analyzerSARIFPath != "" {
		return fmt.Errorf("--analyzer-sarif requires --analyzer-evidence")
	}
	result := assessment.AssessWithOptions(c, files, assessment.AssessmentOptions{EvidenceMode: mode, AnalyzerResults: analyzerResults})
	if err := writeFile(*reportPath, []byte(assessment.MarkdownReport(result))); err != nil {
		return err
	}
	j, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(*jsonPath, j); err != nil {
		return err
	}
	s, err := assessment.SARIFReport(result)
	if err != nil {
		return err
	}
	if err := writeFile(*sarifPath, s); err != nil {
		return err
	}
	fmt.Printf("Verdict: %s; requirements: %d; source files: %d; LLM calls: %d\n", result.Verdict, result.RequirementCount, result.SourceFileCount, result.LLMCalls)
	fmt.Print(assessment.MarkdownReport(result))
	return nil
}

// resolveOpenGrepBinary resolves the exact executable file and records its digest.
// If an expected digest is supplied, a mismatch fails before the analyzer starts.
func resolveOpenGrepBinary(binary, expectedSHA256 string) (string, string, error) {
	resolved, err := analyzer.ResolveExecutable(binary)
	if err != nil {
		return "", "", err
	}
	actual, err := analyzer.HashFile(resolved)
	if err != nil {
		return "", "", fmt.Errorf("hash OpenGrep executable: %w", err)
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
			return "", "", fmt.Errorf("OpenGrep binary SHA-256 mismatch: expected %s but found %s", expected, actual)
		}
	}
	return resolved, actual, nil
}

func runOpenGrepScan(args []string) error {
	fs := flag.NewFlagSet("scan-opengrep", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root to scan")
	config := fs.String("config", "rules/pathfinder-security.yml", "repository-relative OpenGrep rule file")
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
		ConfigurationPath: configRel, ScopePrefixes: []string{"."},
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
	run, err := plan.ExecuteManifest(context.Background(), rootAbs, manifest, plan.OpenGrepTaskRunner{Binary: binaryPath}, cache)
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
			return err
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// ingestionIndex is a reproducible snapshot of source ADR documents. It is
// deliberately separate from requirements.json: parsing is not approval.
type ingestionIndex struct {
	SchemaVersion string               `json:"schema_version"`
	SourceRoot    string               `json:"source_root"`
	Documents     []adringest.Document `json:"documents"`
}

func runIngestADRs(args []string) error {
	fs := flag.NewFlagSet("ingest-adrs", flag.ContinueOnError)
	root := fs.String("root", "adrs", "directory containing ADR Markdown files")
	output := fs.String("json-out", "adr/ingested-adrs.json", "JSON ingestion index output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	docs, err := adringest.ParseDir(*root)
	if err != nil {
		return err
	}
	seen := make(map[string]string, len(docs))
	for _, doc := range docs {
		if previous, ok := seen[doc.ID]; ok {
			return fmt.Errorf("duplicate ADR ID %q in %s and %s", doc.ID, previous, doc.SourcePath)
		}
		seen[doc.ID] = doc.SourcePath
	}
	index := ingestionIndex{
		SchemaVersion: "1",
		SourceRoot:    filepath.ToSlash(filepath.Clean(*root)),
		Documents:     docs,
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ADR ingestion index: %w", err)
	}
	if err := writeFile(*output, data); err != nil {
		return err
	}
	fmt.Printf("Ingested %d ADR document(s) into %s. No obligations were auto-approved.\n", len(docs), *output)
	return nil
}

func runExtractObligations(args []string) error {
	fs := flag.NewFlagSet("extract-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/ingested-adrs.json", "deterministic ADR ingestion index JSON")
	root := fs.String("root", "", "directory containing source ADR Markdown; defaults to source_root from index")
	output := fs.String("json-out", "adr/obligation-candidates.json", "review-only candidate obligation JSON output")
	reportPath := fs.String("report", "", "optional Markdown review queue output")
	minConfidenceReport := fs.Bool("min-confidence-report", false, "print confidence-band summary to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("extract-obligations accepts no positional arguments")
	}
	indexAbs, err := filepath.Abs(*indexPath)
	if err != nil {
		return fmt.Errorf("resolve ingestion index path: %w", err)
	}
	outputAbs, err := filepath.Abs(*output)
	if err != nil {
		return fmt.Errorf("resolve candidate output path: %w", err)
	}
	if filepath.Clean(indexAbs) == filepath.Clean(outputAbs) {
		return fmt.Errorf("candidate output must not overwrite the ADR ingestion index")
	}
	reportAbs := ""
	if *reportPath != "" {
		reportAbs, err = filepath.Abs(*reportPath)
		if err != nil {
			return fmt.Errorf("resolve candidate report path: %w", err)
		}
		if filepath.Clean(reportAbs) == filepath.Clean(indexAbs) || filepath.Clean(reportAbs) == filepath.Clean(outputAbs) {
			return fmt.Errorf("candidate report must not overwrite the ingestion index or candidate JSON output")
		}
	}
	indexBytes, err := os.ReadFile(*indexPath)
	if err != nil {
		return fmt.Errorf("read ADR ingestion index: %w", err)
	}
	var index ingestionIndex
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return fmt.Errorf("decode ADR ingestion index: %w", err)
	}
	if index.SchemaVersion != "1" {
		return fmt.Errorf("unsupported ADR ingestion index schema_version %q", index.SchemaVersion)
	}
	if *root == "" {
		*root = index.SourceRoot
	}
	if strings.TrimSpace(*root) == "" {
		return fmt.Errorf("source root is required via --root or source_root in the index")
	}
	sourceRootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve ADR source root for output safety: %w", err)
	}
	sourceRootReal, err := filepath.EvalSymlinks(sourceRootAbs)
	if err != nil {
		return fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	for _, outputFile := range []string{outputAbs, reportAbs} {
		if outputFile == "" {
			continue
		}
		relative, err := filepath.Rel(sourceRootReal, outputFile)
		if err != nil {
			return fmt.Errorf("validate candidate output containment: %w", err)
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return fmt.Errorf("candidate outputs must remain outside the ADR source root")
		}
	}
	candidates, err := obligations.BuildIndex(index.Documents, *root, index.SourceRoot, indexBytes)
	if err != nil {
		return fmt.Errorf("build review-only obligation candidates: %w", err)
	}
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return fmt.Errorf("encode obligation candidate index: %w", err)
	}
	if *reportPath != "" {
		if err := writeFile(*reportPath, []byte(obligations.MarkdownCandidateReport(candidates))); err != nil {
			return err
		}
	}
	if err := writeFile(*output, data); err != nil {
		return err
	}
	summary := candidates.ConfidenceSummary
	fmt.Printf("Created %d review-required candidate(s) from %d ADR document(s) in %s", candidates.CandidateCount, candidates.DocumentCount, *output)
	if *reportPath != "" {
		fmt.Printf("; Markdown queue: %s", *reportPath)
	}
	fmt.Printf(". No candidate was approved or made enforceable.\n")
	if *minConfidenceReport {
		fmt.Printf("Confidence summary: high=%d medium=%d low=%d (low band is eligible for batch LLM assist)\n",
			summary.High, summary.Medium, summary.Low)
	}
	return nil
}

func runAssistObligations(args []string) error {
	fs := flag.NewFlagSet("assist-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "review-only candidate obligation JSON")
	output := fs.String("json-out", "adr/llm-obligation-proposals.json", "proposal artifact JSON (never mutates catalog/registry)")
	mdOut := fs.String("md-out", "", "optional Markdown proposal report")
	maxConfidence := fs.String("max-confidence", "low", "escalate candidates at or below this confidence band")
	maxCandidates := fs.Int("max-candidates", 50, "spend guard: maximum candidates to assist in one run")
	providerName := fs.String("provider", "offline", "assist provider: offline|openai-compatible")
	model := fs.String("model", "", "model id (required for openai-compatible; optional label for offline)")
	endpoint := fs.String("endpoint", "", "OpenAI-compatible chat completions endpoint")
	apiKeyEnv := fs.String("api-key-env", "LLM_API_KEY", "environment variable holding the API key for live assist")
	maxTokens := fs.Int("max-tokens", 512, "spend guard: max tokens per live completion")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("assist-obligations accepts no positional arguments")
	}
	indexBytes, err := os.ReadFile(*indexPath)
	if err != nil {
		return fmt.Errorf("read candidate index: %w", err)
	}
	var index obligations.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return fmt.Errorf("decode candidate index: %w", err)
	}
	var provider obligations.AssistProvider
	switch strings.ToLower(strings.TrimSpace(*providerName)) {
	case "offline", "offline-fixture":
		provider = obligations.OfflineAssistProvider{ModelID: *model}
	case "openai-compatible":
		apiKey := strings.TrimSpace(os.Getenv(*apiKeyEnv))
		if apiKey == "" {
			return fmt.Errorf("live assist requires API key in $%s (or pass --api-key-env)", *apiKeyEnv)
		}
		if strings.TrimSpace(*model) == "" {
			return fmt.Errorf("--model is required for openai-compatible provider")
		}
		provider = obligations.OpenAICompatProvider{
			Endpoint:  *endpoint,
			APIKey:    apiKey,
			ModelID:   *model,
			MaxTokens: *maxTokens,
		}
	default:
		return fmt.Errorf("unsupported provider %q (want offline|openai-compatible)", *providerName)
	}
	out, err := obligations.AssistObligations(context.Background(), index, obligations.AssistOptions{
		MaxConfidence: *maxConfidence,
		MaxCandidates: *maxCandidates,
		Provider:      provider,
	})
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proposal index: %w", err)
	}
	if err := writeFile(*output, append(data, '\n')); err != nil {
		return err
	}
	if *mdOut != "" {
		if err := writeFile(*mdOut, []byte(obligations.MarkdownProposalReport(out))); err != nil {
			return err
		}
	}
	fmt.Printf("Wrote %d proposal(s) from %d escalated candidate(s) to %s (provider=%s model=%s). Proposals are not approvals and do not mutate the catalog or registry.\n",
		out.ProposalCount, out.CandidateCount, *output, out.Provider, out.Model)
	if *mdOut != "" {
		fmt.Printf("Markdown report: %s\n", *mdOut)
	}
	return nil
}

func runWorkshopEmit(args []string) error {
	fs := flag.NewFlagSet("workshop-emit", flag.ContinueOnError)
	outDir := fs.String("out-dir", "workshop-snapshot", "workshop snapshot output directory")
	runID := fs.String("run-id", "", "workflow run id")
	workflow := fs.String("workflow", "local", "workflow name")
	gitSHA := fs.String("git-sha", "", "git commit SHA")
	htmlURL := fs.String("html-url", "", "workflow run HTML URL")
	providerMode := fs.String("provider-mode", "", "offline|live assist mode tag")
	candidates := fs.String("candidates", "", "obligation candidates JSON")
	coverage := fs.String("coverage", "", "obligation review coverage JSON")
	assessmentPath := fs.String("assessment", "", "assessment JSON")
	proposals := fs.String("proposals", "", "LLM assist proposals JSON")
	harness := fs.String("harness", "", "reuse/corpus harness metrics JSON")
	promptfooPath := fs.String("promptfoo", "", "Promptfoo results JSON")
	corpusLabels := fs.String("corpus-labels", "", "corpus labels.json")
	enforceAssessLLM := fs.Bool("enforce-assess-llm", false, "fail if assessment runtime_llm_calls != 0")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("workshop-emit accepts no positional arguments")
	}
	latest, err := workshop.Emit(workshop.EmitOptions{
		OutDir: *outDir,
		Run: workshop.RunIdentity{
			RunID:        *runID,
			Workflow:     *workflow,
			GitSHA:       *gitSHA,
			HTMLURL:      *htmlURL,
			ProviderMode: *providerMode,
		},
		CandidatesPath:   *candidates,
		CoveragePath:     *coverage,
		AssessmentPath:   *assessmentPath,
		ProposalsPath:    *proposals,
		HarnessPath:      *harness,
		PromptfooPath:    *promptfooPath,
		CorpusLabelsPath: *corpusLabels,
		EnforceAssessLLM: *enforceAssessLLM,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Workshop snapshot %s written to %s (%s)\n", latest.Run.RunID, *outDir, latest.Summary)
	return nil
}

// runAssertIssueVerdict makes GitHub issue regressions executable rather than
// merely descriptive. Add exactly one [expected:PASS], [expected:FAIL], or
// [expected:INDETERMINATE] marker to the issue title to assert its assessment.
func runAssertIssueVerdict(args []string) error {
	fs := flag.NewFlagSet("assert-issue-verdict", flag.ContinueOnError)
	title := fs.String("title", "", "GitHub issue title")
	assessmentPath := fs.String("assessment", "assessment.json", "assessment JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("--title is required")
	}
	data, err := os.ReadFile(*assessmentPath)
	if err != nil {
		return fmt.Errorf("read assessment output: %w", err)
	}
	var result assessment.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode assessment output: %w", err)
	}
	matched, err := assessment.CheckExpectedVerdict(*title, result)
	if err != nil {
		return err
	}
	if !matched {
		fmt.Println("No expected-verdict marker in issue title; assertion skipped.")
		return nil
	}
	fmt.Printf("Issue regression matched: verdict=%s coverage=%s\n", result.Verdict, result.CoverageStatus)
	return nil
}

// runInitReviewRegistry creates an empty append-only review ledger without
// replacing an existing file. It is explicit so a missing/moved ledger is not
// silently mistaken for a clean review history.
func runInitReviewRegistry(args []string) error {
	fs := flag.NewFlagSet("review-init", flag.ContinueOnError)
	output := fs.String("output", "adr/obligation-reviews.json", "new review registry JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("review-init accepts no positional arguments")
	}
	absolute, err := filepath.Abs(*output)
	if err != nil {
		return fmt.Errorf("resolve registry output: %w", err)
	}
	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf("review registry %s already exists; initialization never overwrites review history", absolute)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect registry output %s: %w", absolute, err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
		return fmt.Errorf("create registry directory: %w", err)
	}
	data, err := json.MarshalIndent(obligations.NewRegistry(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode empty review registry: %w", err)
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("review registry %s already exists; initialization never overwrites review history", absolute)
		}
		return fmt.Errorf("create review registry: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(absolute)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write review registry: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync review registry: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close review registry: %w", err)
	}
	ok = true
	fmt.Printf("Initialized empty review registry at %s. No candidate was approved.\n", absolute)
	return nil
}

func readObligationIndex(path string) (obligations.Index, error) {
	var index obligations.Index
	data, err := os.ReadFile(path)
	if err != nil {
		return index, fmt.Errorf("read candidate index: %w", err)
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return index, fmt.Errorf("decode candidate index: %w", err)
	}
	return index, nil
}

func readReviewProposal(path string) (obligations.ReviewProposal, error) {
	var proposal obligations.ReviewProposal
	data, err := os.ReadFile(path)
	if err != nil {
		return proposal, fmt.Errorf("read review proposal: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return proposal, fmt.Errorf("decode review proposal: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return proposal, fmt.Errorf("decode review proposal: trailing JSON value")
		}
		return proposal, fmt.Errorf("decode review proposal: trailing content: %w", err)
	}
	return proposal, nil
}

func runRecordObligationReview(args []string) error {
	fs := flag.NewFlagSet("record-obligation-review", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "review-only candidate index JSON")
	catalogPath := fs.String("requirements", "adr/requirements.json", "approved requirements catalog JSON")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "existing append-only review registry")
	proposalPath := fs.String("proposal", "", "JSON review proposal")
	coveragePath := fs.String("coverage-out", "", "optional binding coverage JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("record-obligation-review accepts no positional arguments")
	}
	if strings.TrimSpace(*proposalPath) == "" {
		return fmt.Errorf("--proposal is required")
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil {
		return err
	}
	proposal, err := readReviewProposal(*proposalPath)
	if err != nil {
		return err
	}
	next, coverage, err := obligations.RecordReview(index, catalog, registry, proposal)
	if err != nil {
		return fmt.Errorf("record obligation review: %w", err)
	}
	// Persist the validated append-only ledger before publishing its derived summary.
	if err := obligations.SaveRegistry(*registryPath, next); err != nil {
		return err
	}
	if strings.TrimSpace(*coveragePath) != "" {
		data, err := json.MarshalIndent(coverage, "", "  ")
		if err != nil {
			return fmt.Errorf("encode binding coverage: %w", err)
		}
		if err := writeFile(*coveragePath, data); err != nil {
			return err
		}
	}
	fmt.Printf("Recorded %s revision for %s. Blocking bindings: %d; advisory bindings: %d; review required: %d; stale: %d.\n",
		proposal.Decision, proposal.CandidateID, coverage.BlockingBindingCount, coverage.AdvisoryBindingCount, coverage.ReviewRequiredCount, coverage.StaleCount)
	return nil
}

func runValidateObligationReviews(args []string) error {
	fs := flag.NewFlagSet("validate-obligation-reviews", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "latest candidate index JSON")
	catalogPath := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "append-only review registry")
	coveragePath := fs.String("json-out", "", "binding coverage JSON output (default: stdout)")
	strict := fs.Bool("strict", false, "fail on stale approvals or orphaned candidate history")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("validate-obligation-reviews accepts no positional arguments")
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil {
		return err
	}
	coverage, err := obligations.ValidateRegistry(index, catalog, registry)
	if err != nil {
		return fmt.Errorf("validate obligation review registry: %w", err)
	}
	data, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		return fmt.Errorf("encode binding coverage: %w", err)
	}
	if strings.TrimSpace(*coveragePath) != "" {
		if err := writeFile(*coveragePath, data); err != nil {
			return err
		}
	} else {
		fmt.Println(string(data))
	}
	if *strict {
		if _, err := obligations.ValidateActiveRegistry(index, catalog, registry); err != nil {
			return fmt.Errorf("strict obligation review validation failed: %w", err)
		}
	}
	return nil
}

// runAssessObligations joins a repository assessment with reviewed candidate
// obligations. It deliberately consumes repository-mode assessment evidence
// rather than silently rerunning scanners with potentially different inputs.
func runAssessObligations(args []string) error {
	fs := flag.NewFlagSet("assess-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "source-cited candidate index")
	ingestionIndexPath := fs.String("ingestion-index", "adr/ingested-adrs.json", "source ADR ingestion index used to reproduce candidates")
	catalogPath := fs.String("requirements", "adr/requirements.json", "approved requirements catalog")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "append-only review registry")
	assessmentPath := fs.String("assessment", "assessment.json", "JSON result from assess-repo")
	root := fs.String("root", ".", "current repository root for stale-evidence validation")
	jsonPath := fs.String("json-out", "obligation-assessment.json", "per-obligation JSON report output")
	markdownPath := fs.String("report", "obligation-assessment.md", "per-obligation Markdown report output")
	enforceBlocking := fs.Bool("enforce-blocking", false, "return an error unless every approved blocking obligation passes with fresh evidence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("assess-obligations accepts no positional arguments")
	}
	inputs := []string{*indexPath, *catalogPath, *registryPath, *assessmentPath}
	outputs := []string{*jsonPath, *markdownPath}
	seen := map[string]string{}
	for _, input := range inputs {
		absolute, err := filepath.Abs(input)
		if err != nil {
			return fmt.Errorf("resolve input path %q: %w", input, err)
		}
		seen[filepath.Clean(absolute)] = "input"
	}
	for _, output := range outputs {
		absolute, err := filepath.Abs(output)
		if err != nil {
			return fmt.Errorf("resolve output path %q: %w", output, err)
		}
		key := filepath.Clean(absolute)
		if prior, exists := seen[key]; exists {
			return fmt.Errorf("output path %q collides with an existing %s path", output, prior)
		}
		seen[key] = "output"
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	ingestionBytes, err := os.ReadFile(*ingestionIndexPath)
	if err != nil {
		return fmt.Errorf("read ADR ingestion index for candidate freshness: %w", err)
	}
	var sourceIndex ingestionIndex
	if err := json.Unmarshal(ingestionBytes, &sourceIndex); err != nil {
		return fmt.Errorf("decode ADR ingestion index for candidate freshness: %w", err)
	}
	if sourceIndex.SchemaVersion != "1" {
		return fmt.Errorf("unsupported ADR ingestion index schema_version %q", sourceIndex.SchemaVersion)
	}
	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	sourceRootPath := sourceIndex.SourceRoot
	if !filepath.IsAbs(sourceRootPath) {
		sourceRootPath = filepath.Join(rootReal, sourceRootPath)
	}
	sourceRootAbs, err := filepath.Abs(sourceRootPath)
	if err != nil {
		return fmt.Errorf("resolve ADR source root: %w", err)
	}
	sourceRootReal, err := filepath.EvalSymlinks(sourceRootAbs)
	if err != nil {
		return fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	relativeSourceRoot, err := filepath.Rel(rootReal, sourceRootReal)
	if err != nil {
		return fmt.Errorf("validate ADR source root containment: %w", err)
	}
	if relativeSourceRoot == ".." || strings.HasPrefix(relativeSourceRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeSourceRoot) {
		return fmt.Errorf("ADR source root must remain inside the current repository root")
	}
	rebuiltIndex, err := obligations.BuildIndex(sourceIndex.Documents, sourceRootReal, sourceIndex.SourceRoot, ingestionBytes)
	if err != nil {
		return fmt.Errorf("rebuild candidate index from current ADR sources: %w", err)
	}
	expectedIndex, err := json.Marshal(rebuiltIndex)
	if err != nil {
		return fmt.Errorf("encode rebuilt candidate index: %w", err)
	}
	providedIndex, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("encode provided candidate index: %w", err)
	}
	if !bytes.Equal(expectedIndex, providedIndex) {
		return fmt.Errorf("candidate index does not match current ADR sources and ingestion index; rerun ingest-adrs and extract-obligations")
	}
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*assessmentPath)
	if err != nil {
		return fmt.Errorf("read repository assessment: %w", err)
	}
	var evidence assessment.Result
	if err := json.Unmarshal(data, &evidence); err != nil {
		return fmt.Errorf("decode repository assessment: %w", err)
	}
	currentFiles, err := assessment.LoadSourceFiles(*root)
	if err != nil {
		return fmt.Errorf("load current repository source inventory: %w", err)
	}
	currentSourceSnapshot := assessment.HashSourceFiles(currentFiles)
	currentRepositorySnapshot, err := analyzer.HashInputTree(*root)
	if err != nil {
		return fmt.Errorf("hash current repository snapshot: %w", err)
	}
	report, err := obligations.AssessCandidates(index, catalog, registry, evidence, currentSourceSnapshot, currentRepositorySnapshot)
	if err != nil {
		return fmt.Errorf("assess obligation candidates: %w", err)
	}
	indexBytes, err := os.ReadFile(*indexPath)
	if err != nil {
		return fmt.Errorf("read candidate index for provenance: %w", err)
	}
	registryBytes, err := os.ReadFile(*registryPath)
	if err != nil {
		return fmt.Errorf("read review registry for provenance: %w", err)
	}
	indexDigest := sha256.Sum256(indexBytes)
	registryDigest := sha256.Sum256(registryBytes)
	assessmentDigest := sha256.Sum256(data)
	report.CandidateIndexArtifactSHA256 = hex.EncodeToString(indexDigest[:])
	report.ReviewRegistryArtifactSHA256 = hex.EncodeToString(registryDigest[:])
	report.AssessmentArtifactSHA256 = hex.EncodeToString(assessmentDigest[:])
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode obligation assessment: %w", err)
	}
	if err := writeFile(*jsonPath, encoded); err != nil {
		return err
	}
	if err := writeFile(*markdownPath, []byte(obligations.MarkdownAssessmentReport(report))); err != nil {
		return err
	}
	fmt.Printf("Obligation assessment: %d candidate(s); verdict=%s; PASS=%d FAIL=%d INDETERMINATE=%d NOT_APPLICABLE=%d; JSON=%s; report=%s\n",
		report.CandidateCount, report.ObligationVerdict, report.Counts["PASS"], report.Counts["FAIL"], report.Counts["INDETERMINATE"],
		report.Counts["NOT_APPLICABLE"], *jsonPath, *markdownPath)
	if *enforceBlocking {
		if err := validateBlockingGate(report); err != nil {
			return fmt.Errorf("%w; see %s", err, *markdownPath)
		}
	}
	return nil
}

// runPlanRepo creates a deterministic, source-bound execution plan. Planning
// does not launch analyzers; every execution must re-verify the manifest before
// using its cache keys or dispatching tasks.
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

// runSpecificationCheck orchestrates the deterministic source-to-evidence
// lifecycle. It always regenerates ingestion/candidate artifacts, so source
// drift cannot be hidden by a stale checked-in candidate index.
func runSpecificationCheck(args []string) error {
	fs := flag.NewFlagSet("specification-check", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	var sourceRoot string
	fs.StringVar(&sourceRoot, "source-root", "adrs", "repository-relative directory containing ADR or plain-English specification Markdown")
	fs.StringVar(&sourceRoot, "adr-root", "adrs", "legacy alias for --source-root")
	requirements := fs.String("requirements", "adr/requirements.json", "accepted requirements catalog JSON")
	registry := fs.String("registry", "adr/obligation-reviews.json", "append-only obligation review registry JSON")
	analyzerEvidence := fs.String("analyzer-evidence", "", "optional normalized analyzer scan evidence JSON")
	analyzerSARIF := fs.String("analyzer-sarif", "", "raw SARIF paired with normalized analyzer evidence")
	reportOnly := fs.Bool("report-only", false, "write assessment artifacts without enforcing blocking obligations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("specification-check accepts no positional arguments")
	}
	if (*analyzerEvidence == "") != (*analyzerSARIF == "") {
		return fmt.Errorf("--analyzer-evidence and --analyzer-sarif must be provided together")
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolveInput := func(raw string) string {
		if filepath.IsAbs(raw) {
			return filepath.Clean(raw)
		}
		return filepath.Join(rootReal, raw)
	}
	adrRootAbs, err := filepath.Abs(resolveInput(sourceRoot))
	if err != nil {
		return fmt.Errorf("resolve ADR source root: %w", err)
	}
	adrRootReal, err := filepath.EvalSymlinks(adrRootAbs)
	if err != nil {
		return fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	relativeADRRoot, err := filepath.Rel(rootReal, adrRootReal)
	if err != nil {
		return fmt.Errorf("validate ADR source root containment: %w", err)
	}
	if relativeADRRoot == ".." || strings.HasPrefix(relativeADRRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeADRRoot) {
		return fmt.Errorf("ADR source root must remain inside repository root")
	}
	artifactDir := filepath.Join(rootReal, "adr")
	if resolved, resolveErr := filepath.EvalSymlinks(artifactDir); resolveErr == nil {
		artifactDir = resolved
	} else if !os.IsNotExist(resolveErr) {
		return fmt.Errorf("resolve generated artifact directory: %w", resolveErr)
	}
	relativeArtifactDir, err := filepath.Rel(rootReal, artifactDir)
	if err != nil || relativeArtifactDir == ".." || strings.HasPrefix(relativeArtifactDir, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeArtifactDir) {
		return fmt.Errorf("generated artifact directory must remain inside repository root")
	}
	relArtifacts, err := filepath.Rel(adrRootReal, artifactDir)
	if err != nil {
		return fmt.Errorf("validate generated artifact directory: %w", err)
	}
	if relArtifacts == "." || (relArtifacts != ".." && !strings.HasPrefix(relArtifacts, ".."+string(filepath.Separator))) {
		return fmt.Errorf("ADR source root must not contain the generated artifact directory %q", artifactDir)
	}

	catalogPath, registryPath := resolveInput(*requirements), resolveInput(*registry)
	ingestionPath := filepath.Join(artifactDir, "ingested-adrs.json")
	candidatePath := filepath.Join(artifactDir, "obligation-candidates.json")
	candidateReportPath := filepath.Join(artifactDir, "obligation-candidates.md")
	assessmentPath := filepath.Join(rootReal, "assessment.json")
	repositoryReportPath := filepath.Join(rootReal, "report.md")
	sarifPath := filepath.Join(rootReal, "results.sarif")
	obligationReportPath := filepath.Join(rootReal, "obligation-assessment.json")
	obligationMarkdownPath := filepath.Join(rootReal, "obligation-assessment.md")
	outputs := []string{ingestionPath, candidatePath, candidateReportPath, assessmentPath, repositoryReportPath, sarifPath, obligationReportPath, obligationMarkdownPath}
	canonicalOutputs := make([]string, len(outputs))
	outputOwners := make(map[string]string, len(outputs))
	for i, output := range outputs {
		canonicalOutput := canonicalPathForCollision(output)
		relativeOutput, err := filepath.Rel(rootReal, canonicalOutput)
		if err != nil || relativeOutput == ".." ||
			strings.HasPrefix(relativeOutput, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(relativeOutput) {
			return fmt.Errorf("generated output %q resolves outside repository root", output)
		}
		if previous, exists := outputOwners[canonicalOutput]; exists {
			return fmt.Errorf("generated output paths %q and %q resolve to the same target", previous, output)
		}
		outputOwners[canonicalOutput] = output
		canonicalOutputs[i] = canonicalOutput
	}

	inputs := []string{catalogPath, registryPath}
	if *analyzerEvidence != "" {
		inputs = append(inputs, resolveInput(*analyzerEvidence), resolveInput(*analyzerSARIF))
	}
	for _, input := range inputs {
		canonicalInput := canonicalPathForCollision(input)
		for i, output := range outputs {
			if canonicalInput == canonicalOutputs[i] {
				return fmt.Errorf("input path %q collides with generated output %q", input, output)
			}
		}
	}

	if err := runIngestADRs([]string{"--root", adrRootReal, "--json-out", ingestionPath}); err != nil {
		return fmt.Errorf("ingest ADR sources: %w", err)
	}
	if err := runExtractObligations([]string{"--index", ingestionPath, "--root", adrRootReal, "--json-out", candidatePath, "--report", candidateReportPath}); err != nil {
		return fmt.Errorf("extract obligation candidates: %w", err)
	}
	assessArgs := []string{"--root", rootReal, "--requirements", catalogPath, "--report", repositoryReportPath, "--json-out", assessmentPath, "--sarif-out", sarifPath}
	if *analyzerEvidence != "" {
		assessArgs = append(assessArgs, "--analyzer-evidence", resolveInput(*analyzerEvidence), "--analyzer-sarif", resolveInput(*analyzerSARIF))
	}
	if err := runAssess(assessArgs, true); err != nil {
		return fmt.Errorf("assess repository: %w", err)
	}
	joinArgs := []string{
		"--root", rootReal, "--index", candidatePath, "--ingestion-index", ingestionPath,
		"--requirements", catalogPath, "--registry", registryPath, "--assessment", assessmentPath,
		"--json-out", obligationReportPath, "--report", obligationMarkdownPath,
	}
	if !*reportOnly {
		joinArgs = append(joinArgs, "--enforce-blocking")
	}
	if err := runAssessObligations(joinArgs); err != nil {
		return fmt.Errorf("specification check failed: %w", err)
	}
	return nil
}

func validateBlockingGate(report obligations.AssessmentReport) error {
	if report.BindingCoverage.OrphanedCount > 0 {
		return fmt.Errorf("%d orphaned review history record(s) must be explicitly superseded before enforcing blocking obligations", report.BindingCoverage.OrphanedCount)
	}
	if report.BindingCoverage.StaleCount > 0 {
		return fmt.Errorf("%d stale candidate review or verifier binding snapshot(s) must be re-reviewed before enforcing blocking obligations", report.BindingCoverage.StaleCount)
	}
	for _, candidate := range report.Candidates {
		// An unreviewed or source-stale candidate may represent a newly
		// introduced blocking obligation. Do not let absence of an approval
		// silently bypass the enforcement gate.
		if candidate.Decision == obligations.DecisionPending {
			return fmt.Errorf("obligation candidate %s remains pending review (status=%s); review or explicitly reject it before enforcing blocking obligations", candidate.CandidateID, candidate.Status)
		}
		if candidate.Decision == obligations.DecisionApproved && candidate.EnforcementMode == obligations.EnforcementBlocking && candidate.Status != "PASS" {
			return fmt.Errorf("blocking obligation %s did not pass (status=%s, evidence=%s)", candidate.CandidateID, candidate.Status, candidate.EvidenceStatus)
		}
	}
	return nil
}

func runPlanRepo(args []string) error {
	fs := flag.NewFlagSet("plan-repo", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	analyzerName := fs.String("analyzer", "opengrep", "analyzer adapter name")
	version := fs.String("version", "1.30.0", "exact analyzer version")
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

func runExecutePlan(args []string) error {
	return runExecutePlanWithRunner(args, nil)
}

// runExecutePlanWithRunner supports deterministic CLI integration tests while
// keeping production runner selection explicit and pinned by the plan manifest.
func runExecutePlanWithRunner(args []string, injected plan.TaskRunner) error {
	fs := flag.NewFlagSet("execute-plan", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	manifestPath := fs.String("manifest", "analyzer-evidence/plan.json", "source-bound plan manifest")
	binary := fs.String("binary", "opengrep", "OpenGrep executable path")
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
		runner = plan.DefaultRunnerRegistry(*binary)
	}
	result, runErr := plan.ExecuteManifest(context.Background(), *root, manifest, runner, cache)
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
