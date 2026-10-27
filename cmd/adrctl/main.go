package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
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
	case "assert-issue-verdict":
		err = runAssertIssueVerdict(os.Args[2:])
	case "review-init":
		err = runInitReviewRegistry(os.Args[2:])
	case "record-obligation-review":
		err = runRecordObligationReview(os.Args[2:])
	case "validate-obligation-reviews":
		err = runValidateObligationReviews(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "usage: adrctl <validate|assess|assess-repo|ingest-adrs|extract-obligations|scan-opengrep|assert-issue-verdict|review-init|record-obligation-review|validate-obligation-reviews|version> [flags]")
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


// runInitReviewRegistry creates an empty append-only review ledger without
// replacing an existing file. It is explicit so a missing/moved ledger is not
// silently mistaken for a clean review history.
func runInitReviewRegistry(args []string) error {
	fs := flag.NewFlagSet("review-init", flag.ContinueOnError)
	output := fs.String("output", "adr/obligation-reviews.json", "new review registry JSON")
	if err := fs.Parse(args); err != nil { return err }
	if fs.NArg() != 0 { return fmt.Errorf("review-init accepts no positional arguments") }
	absolute, err := filepath.Abs(*output)
	if err != nil { return fmt.Errorf("resolve registry output: %w", err) }
	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf("review registry %s already exists; initialization never overwrites review history", absolute)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect registry output %s: %w", absolute, err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
		return fmt.Errorf("create registry directory: %w", err)
	}
	data, err := json.MarshalIndent(obligations.NewRegistry(), "", "  ")
	if err != nil { return fmt.Errorf("encode empty review registry: %w", err) }
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) { return fmt.Errorf("review registry %s already exists; initialization never overwrites review history", absolute) }
		return fmt.Errorf("create review registry: %w", err)
	}
	ok := false
	defer func() { if !ok { _ = os.Remove(absolute) } }()
	if _, err := file.Write(data); err != nil { _ = file.Close(); return fmt.Errorf("write review registry: %w", err) }
	if err := file.Sync(); err != nil { _ = file.Close(); return fmt.Errorf("sync review registry: %w", err) }
	if err := file.Close(); err != nil { return fmt.Errorf("close review registry: %w", err) }
	ok = true
	fmt.Printf("Initialized empty review registry at %s. No candidate was approved.\n", absolute)
	return nil
}

func readObligationIndex(path string) (obligations.Index, error) {
	var index obligations.Index
	data, err := os.ReadFile(path)
	if err != nil { return index, fmt.Errorf("read candidate index: %w", err) }
	if err := json.Unmarshal(data, &index); err != nil { return index, fmt.Errorf("decode candidate index: %w", err) }
	return index, nil
}

func readReviewProposal(path string) (obligations.ReviewProposal, error) {
	var proposal obligations.ReviewProposal
	data, err := os.ReadFile(path)
	if err != nil { return proposal, fmt.Errorf("read review proposal: %w", err) }
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil { return proposal, fmt.Errorf("decode review proposal: %w", err) }
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return proposal, fmt.Errorf("decode review proposal: trailing JSON value")
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
	if err := fs.Parse(args); err != nil { return err }
	if fs.NArg() != 0 { return fmt.Errorf("record-obligation-review accepts no positional arguments") }
	if strings.TrimSpace(*proposalPath) == "" { return fmt.Errorf("--proposal is required") }
	index, err := readObligationIndex(*indexPath)
	if err != nil { return err }
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil { return err }
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil { return err }
	proposal, err := readReviewProposal(*proposalPath)
	if err != nil { return err }
	next, coverage, err := obligations.RecordReview(index, catalog, registry, proposal)
	if err != nil { return fmt.Errorf("record obligation review: %w", err) }
	// Persist the validated append-only ledger before publishing its derived summary.
	if err := obligations.SaveRegistry(*registryPath, next); err != nil { return err }
	if strings.TrimSpace(*coveragePath) != "" {
		data, err := json.MarshalIndent(coverage, "", "  ")
		if err != nil { return fmt.Errorf("encode binding coverage: %w", err) }
		if err := writeFile(*coveragePath, data); err != nil { return err }
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
	if err := fs.Parse(args); err != nil { return err }
	if fs.NArg() != 0 { return fmt.Errorf("validate-obligation-reviews accepts no positional arguments") }
	index, err := readObligationIndex(*indexPath)
	if err != nil { return err }
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil { return err }
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil { return err }
	coverage, err := obligations.ValidateRegistry(index, catalog, registry)
	if err != nil { return fmt.Errorf("validate obligation review registry: %w", err) }
	data, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil { return fmt.Errorf("encode binding coverage: %w", err) }
	if strings.TrimSpace(*coveragePath) != "" {
		if err := writeFile(*coveragePath, data); err != nil { return err }
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
