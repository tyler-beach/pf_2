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

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
	"github.com/tyler-beach/pf_2/internal/plan"
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
	fmt.Fprintln(os.Stderr, "usage: adrctl <validate|assess|assess-repo|ingest-adrs|extract-obligations|scan-opengrep|plan-repo|verify-plan|execute-plan|assert-issue-verdict|version> [flags]")
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
		if !repoMode { return fmt.Errorf("--analyzer-evidence is supported only for repository assessments") }
		if *analyzerSARIFPath == "" { return fmt.Errorf("--analyzer-sarif is required when --analyzer-evidence is provided") }
		data, readErr := os.ReadFile(*analyzerEvidencePath)
		if readErr != nil { return fmt.Errorf("read analyzer evidence: %w", readErr) }
		var evidence analyzer.ScanResult
		if decodeErr := json.Unmarshal(data, &evidence); decodeErr != nil { return fmt.Errorf("decode analyzer evidence: %w", decodeErr) }
		rawSARIF, readErr := os.ReadFile(*analyzerSARIFPath)
		if readErr != nil { return fmt.Errorf("read raw analyzer SARIF: %w", readErr) }
		verified, verifyErr := analyzer.VerifyScanEvidence(*root, evidence, rawSARIF)
		if verifyErr != nil { return fmt.Errorf("verify analyzer evidence: %w", verifyErr) }
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

func runOpenGrepScan(args []string) error {
	fs := flag.NewFlagSet("scan-opengrep", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root to scan")
	config := fs.String("config", "rules/pathfinder-security.yml", "repository-relative OpenGrep rule file")
	binary := fs.String("binary", "opengrep", "OpenGrep executable path")
	version := fs.String("version", "1.30.0", "expected pinned OpenGrep version")
	timeout := fs.Duration("timeout", 5*time.Minute, "scan timeout (for example 90s or 5m)")
	jsonPath := fs.String("json-out", "analyzer-evidence/opengrep.json", "normalized analyzer evidence output")
	sarifPath := fs.String("sarif-out", "analyzer-evidence/opengrep.sarif", "raw SARIF output for audit and troubleshooting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	scanner := analyzer.OpenGrepScanner{
		Binary: *binary,
		Version: *version,
		ConfigPath: *config,
		Timeout: *timeout,
	}
	result, scanErr := scanner.Scan(context.Background(), *root)
	data, encodeErr := analyzer.WriteJSON(result)
	if encodeErr != nil {
		return encodeErr
	}
	if err := writeFile(*jsonPath, data); err != nil {
		return err
	}
	if len(result.RawSARIF) > 0 {
		if err := writeFile(*sarifPath, result.RawSARIF); err != nil { return err }
	}
	if scanErr != nil {
		return fmt.Errorf("OpenGrep assessment failed; evidence written to %s: %w", *jsonPath, scanErr)
	}
	fmt.Printf("OpenGrep %s: %d finding(s); evidence: %s; SARIF: %s; configuration SHA-256: %s; input SHA-256: %s\n",
		*version, len(result.Findings), *jsonPath, *sarifPath, result.ConfigurationSHA256, result.InputSHA256)
	return nil
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
	if err := fs.Parse(args); err != nil { return err }
	if fs.NArg() != 0 { return fmt.Errorf("extract-obligations accepts no positional arguments") }
	indexAbs, err := filepath.Abs(*indexPath)
	if err != nil { return fmt.Errorf("resolve ingestion index path: %w", err) }
	outputAbs, err := filepath.Abs(*output)
	if err != nil { return fmt.Errorf("resolve candidate output path: %w", err) }
	if filepath.Clean(indexAbs) == filepath.Clean(outputAbs) {
		return fmt.Errorf("candidate output must not overwrite the ADR ingestion index")
	}
	indexBytes, err := os.ReadFile(*indexPath)
	if err != nil { return fmt.Errorf("read ADR ingestion index: %w", err) }
	var index ingestionIndex
	if err := json.Unmarshal(indexBytes, &index); err != nil { return fmt.Errorf("decode ADR ingestion index: %w", err) }
	if index.SchemaVersion != "1" { return fmt.Errorf("unsupported ADR ingestion index schema_version %q", index.SchemaVersion) }
	if *root == "" { *root = index.SourceRoot }
	if strings.TrimSpace(*root) == "" { return fmt.Errorf("source root is required via --root or source_root in the index") }
	candidates, err := obligations.BuildIndex(index.Documents, *root, index.SourceRoot, indexBytes)
	if err != nil { return fmt.Errorf("build review-only obligation candidates: %w", err) }
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil { return fmt.Errorf("encode obligation candidate index: %w", err) }
	if err := writeFile(*output, data); err != nil { return err }
	fmt.Printf("Created %d review-required candidate(s) from %d ADR document(s) in %s. No candidate was approved or made enforceable.\n",
		candidates.CandidateCount, candidates.DocumentCount, *output)
	return nil
}


// runAssertIssueVerdict makes GitHub issue regressions executable rather than
// merely descriptive. Add exactly one [expected:PASS], [expected:FAIL], or
// [expected:INDETERMINATE] marker to the issue title to assert its assessment.
func runAssertIssueVerdict(args []string) error {
	fs := flag.NewFlagSet("assert-issue-verdict", flag.ContinueOnError)
	title := fs.String("title", "", "GitHub issue title")
	assessmentPath := fs.String("assessment", "assessment.json", "assessment JSON output")
	if err := fs.Parse(args); err != nil { return err }
	if strings.TrimSpace(*title) == "" { return fmt.Errorf("--title is required") }
	data, err := os.ReadFile(*assessmentPath)
	if err != nil { return fmt.Errorf("read assessment output: %w", err) }
	var result assessment.Result
	if err := json.Unmarshal(data, &result); err != nil { return fmt.Errorf("decode assessment output: %w", err) }
	matched, err := assessment.CheckExpectedVerdict(*title, result)
	if err != nil { return err }
	if !matched {
		fmt.Println("No expected-verdict marker in issue title; assertion skipped.")
		return nil
	}
	fmt.Printf("Issue regression matched: verdict=%s coverage=%s\n", result.Verdict, result.CoverageStatus)
	return nil
}


// runPlanRepo creates a deterministic, source-bound execution plan. Planning
// does not launch analyzers; every execution must re-verify the manifest before
// using its cache keys or dispatching tasks.
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
	if err := fs.Parse(args); err != nil { return err }
	if fs.NArg() != 0 { return fmt.Errorf("plan-repo accepts no positional arguments") }
	if *timeout < time.Millisecond || *backoff < 0 || *maxBackoff < 0 {
		return fmt.Errorf("timeouts and retry backoffs must not be negative, and timeout must be at least 1ms")
	}
	scopePrefixes := []string{}
	for _, item := range strings.Split(*scope, ",") {
		item = strings.TrimSpace(item)
		if item != "" { scopePrefixes = append(scopePrefixes, item) }
	}
	spec := plan.AnalyzerSpec{
		Name: *analyzerName, Version: *version, BinarySHA256: *binarySHA256, RuleSetSHA256: *ruleSetSHA256,
		ConfigurationPath: *config, ScopePrefixes: scopePrefixes, TimeoutMillis: timeout.Milliseconds(), MaxRetries: *maxRetries,
		MaxOutputBytes: *maxOutputBytes, RetryBackoffMillis: backoff.Milliseconds(),
		MaxRetryBackoffMillis: maxBackoff.Milliseconds(),
		RetryableFailureKinds: []string{"timeout", "temporary_process_start"},
	}
	manifest, err := plan.BuildManifest(*root, []plan.AnalyzerSpec{spec}, *maxConcurrency)
	if err != nil { return err }
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil { return fmt.Errorf("encode analyzer plan manifest: %w", err) }
	data = append(data, '\n')
	if err := writeFile(*output, data); err != nil { return err }
	fmt.Printf("Plan %s; input %s; tasks %d; concurrency %d; output %s. No analyzer was executed.\n",
		manifest.ManifestSHA256, manifest.InputSHA256, len(manifest.Tasks), manifest.MaxConcurrency, *output)
	return nil
}

func readPlanManifest(path string) (plan.Manifest, error) {
	var manifest plan.Manifest
	data, err := os.ReadFile(path)
	if err != nil { return manifest, fmt.Errorf("read plan manifest: %w", err) }
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil { return manifest, fmt.Errorf("decode plan manifest: %w", err) }
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil { return manifest, fmt.Errorf("decode plan manifest: trailing JSON value") }
		return manifest, fmt.Errorf("decode plan manifest: trailing content: %w", err)
	}
	return manifest, nil
}

func runVerifyPlan(args []string) error {
	fs := flag.NewFlagSet("verify-plan", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	manifestPath := fs.String("manifest", "analyzer-evidence/plan.json", "plan manifest JSON")
	if err := fs.Parse(args); err != nil { return err }
	if fs.NArg() != 0 { return fmt.Errorf("verify-plan accepts no positional arguments") }
	manifest, err := readPlanManifest(*manifestPath)
	if err != nil { return err }
	if err := plan.VerifyManifest(*root, manifest); err != nil { return err }
	fmt.Printf("Plan verified: %s; input %s; tasks %d.\n", manifest.ManifestSHA256, manifest.InputSHA256, len(manifest.Tasks))
	return nil
}


func runExecutePlan(args []string) error {
	return runExecutePlanWithRunner(args,nil)
}

// runExecutePlanWithRunner supports deterministic CLI integration tests while
// keeping production runner selection explicit and pinned by the plan manifest.
func runExecutePlanWithRunner(args []string, injected plan.TaskRunner) error {
	fs:=flag.NewFlagSet("execute-plan",flag.ContinueOnError)
	root:=fs.String("root",".","repository root")
	manifestPath:=fs.String("manifest","analyzer-evidence/plan.json","source-bound plan manifest")
	binary:=fs.String("binary","opengrep","OpenGrep executable path")
	cacheDirDefault,cacheErr:=os.UserCacheDir()
	if cacheErr!=nil{cacheDirDefault=os.TempDir()}
	cacheDir:=fs.String("cache-dir",filepath.Join(cacheDirDefault,"pathfinder","analyzers"),"private local cache directory (prefer outside repository root)")
	output:=fs.String("json-out","analyzer-evidence/execution.json","execution result manifest JSON")
	sarifDir:=fs.String("sarif-dir","analyzer-evidence/raw","directory for per-task raw SARIF artifacts")
	if err:=fs.Parse(args);err!=nil{return err}
	if fs.NArg()!=0{return fmt.Errorf("execute-plan accepts no positional arguments")}
	manifest,err:=readPlanManifest(*manifestPath)
	if err!=nil{return err}
	if err:=plan.VerifyManifest(*root,manifest);err!=nil{return fmt.Errorf("verify plan before execution: %w",err)}
	cache,err:=plan.NewFileCache(*cacheDir)
	if err!=nil{return err}
	runner:=injected
	if runner==nil{runner=plan.OpenGrepTaskRunner{Binary:*binary}}
	result,runErr:=plan.ExecuteManifest(context.Background(),*root,manifest,runner,cache)
	for i:=range result.Tasks{
		task:=&result.Tasks[i]
		if len(task.RawSARIF)==0{continue}
		path:=filepath.Join(*sarifDir,task.TaskID+".sarif")
		if err:=writeFile(path,task.RawSARIF);err!=nil{
			task.Status=plan.TaskStatusError
			task.Error="write raw SARIF artifact: "+err.Error()
			result.Status=plan.RunStatusPartial
			if runErr==nil{runErr=err}
			continue
		}
		task.RawSARIFPath=path
	}
	data,err:=json.MarshalIndent(result,"","  ")
	if err!=nil{return fmt.Errorf("encode execution result manifest: %w",err)}
	if err:=writeFile(*output,append(data,'\n'));err!=nil{return err}
	fmt.Printf("Execution %s; plan %s; tasks %d; output %s.\n",result.Status,result.PlanManifestSHA256,len(result.Tasks),*output)
	if runErr!=nil{return runErr}
	if result.Status!=plan.RunStatusComplete{return fmt.Errorf("execution did not complete successfully; status %s; inspect %s",result.Status,*output)}
	return nil
}
