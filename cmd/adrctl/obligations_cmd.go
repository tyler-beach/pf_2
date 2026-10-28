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
)

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

func runAssistObligations(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("assist-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "review-only candidate obligation JSON")
	root := fs.String("root", "", "ADR source root for citation byte checks (defaults to candidate path common parent or cwd)")
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
	sourceRoot := strings.TrimSpace(*root)
	if sourceRoot == "" {
		sourceRoot = "."
	}
	sourceRootAbs, err := filepath.Abs(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
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
	out, err := obligations.AssistObligations(ctx, index, obligations.AssistOptions{
		MaxConfidence: *maxConfidence,
		MaxCandidates: *maxCandidates,
		SourceRoot:    sourceRootAbs,
		Provider:      provider,
	})
	if err != nil {
		return fmt.Errorf("assist obligations: %w", err)
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

func runProposalToReview(args []string) error {
	fs := flag.NewFlagSet("proposal-to-review", flag.ContinueOnError)
	proposalsPath := fs.String("proposals", "adr/llm-obligation-proposals.json", "assist proposal index JSON")
	proposalID := fs.String("proposal-id", "", "proposal id to convert")
	decision := fs.String("decision", "approved", "approved|rejected")
	reviewer := fs.String("reviewer", "", "reviewer identity")
	reference := fs.String("review-reference", "", "review reference URL or note")
	rationale := fs.String("rationale", "", "optional rationale override")
	enforcement := fs.String("enforcement-mode", obligations.EnforcementReviewRequired, "enforcement mode")
	jsonOut := fs.String("json-out", "", "write ReviewProposal JSON (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("proposal-to-review accepts no positional arguments")
	}
	if strings.TrimSpace(*proposalID) == "" || strings.TrimSpace(*jsonOut) == "" {
		return fmt.Errorf("--proposal-id and --json-out are required")
	}
	if strings.TrimSpace(*reviewer) == "" || strings.TrimSpace(*reference) == "" {
		return fmt.Errorf("--reviewer and --review-reference are required")
	}
	index, err := obligations.LoadProposalIndex(*proposalsPath)
	if err != nil {
		return err
	}
	var found obligations.Proposal
	ok := false
	for _, p := range index.Proposals {
		if p.ProposalID == *proposalID {
			found = p
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("proposal %s not found", *proposalID)
	}
	review, err := obligations.ProposalToReview(found, *decision, *reviewer, *reference, *rationale, *enforcement, time.Now())
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(*jsonOut, append(data, '\n')); err != nil {
		return err
	}
	fmt.Printf("Wrote review proposal for %s (%s) to %s\n", found.ProposalID, review.Decision, *jsonOut)
	return nil
}

func runAcceptAssistProposal(args []string) error {
	fs := flag.NewFlagSet("accept-assist-proposal", flag.ContinueOnError)
	proposalsPath := fs.String("proposals", "adr/llm-obligation-proposals.json", "assist proposal index JSON")
	proposalID := fs.String("proposal-id", "", "single proposal id")
	allPending := fs.Bool("all-pending", false, "accept/reject all pending proposals (up to --max)")
	maxN := fs.Int("max", 25, "max proposals when using --all-pending")
	decision := fs.String("decision", "approved", "approved|rejected")
	indexPath := fs.String("index", "adr/obligation-candidates.json", "candidate index JSON")
	catalogPath := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "append-only review registry")
	reviewer := fs.String("reviewer", "", "reviewer identity")
	reference := fs.String("review-reference", "", "review reference URL or note")
	rationale := fs.String("rationale", "", "optional rationale override")
	enforcement := fs.String("enforcement-mode", obligations.EnforcementReviewRequired, "enforcement mode")
	coverageOut := fs.String("coverage-out", "", "optional binding coverage JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("accept-assist-proposal accepts no positional arguments")
	}
	if strings.TrimSpace(*reviewer) == "" || strings.TrimSpace(*reference) == "" {
		return fmt.Errorf("--reviewer and --review-reference are required")
	}
	if !*allPending && strings.TrimSpace(*proposalID) == "" {
		return fmt.Errorf("--proposal-id is required unless --all-pending is set")
	}
	candIndex, err := readObligationIndex(*indexPath)
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
	proposals, err := obligations.LoadProposalIndex(*proposalsPath)
	if err != nil {
		return err
	}
	ids := []string{}
	if *allPending {
		for _, p := range proposals.Proposals {
			if p.ApprovalStatus == obligations.DecisionPending && !p.Executable {
				ids = append(ids, p.ProposalID)
			}
			if len(ids) >= *maxN {
				break
			}
		}
	} else {
		ids = []string{*proposalID}
	}
	if len(ids) == 0 {
		return fmt.Errorf("no proposals selected")
	}
	var coverage obligations.BindingCoverage
	for _, id := range ids {
		next, cov, _, err := obligations.AcceptAssistProposal(candIndex, catalog, registry, proposals, id, *decision, *reviewer, *reference, *rationale, *enforcement, time.Now())
		if err != nil {
			return fmt.Errorf("accept %s: %w", id, err)
		}
		registry = next
		coverage = cov
		fmt.Printf("Recorded %s for assist proposal %s\n", *decision, id)
	}
	if err := obligations.SaveRegistry(*registryPath, registry); err != nil {
		return err
	}
	if strings.TrimSpace(*coverageOut) != "" {
		data, err := json.MarshalIndent(coverage, "", "  ")
		if err != nil {
			return err
		}
		if err := writeFile(*coverageOut, data); err != nil {
			return err
		}
	}
	fmt.Printf("Updated registry %s (%d proposal(s)). Blocking bindings: %d.\n", *registryPath, len(ids), coverage.BlockingBindingCount)
	return nil
}

func runLowConfidenceChecklist(args []string) error {
	fs := flag.NewFlagSet("low-confidence-checklist", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "candidate index JSON")
	mdOut := fs.String("md-out", "", "Markdown checklist output (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("low-confidence-checklist accepts no positional arguments")
	}
	if strings.TrimSpace(*mdOut) == "" {
		return fmt.Errorf("--md-out is required")
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	body := obligations.LowConfidenceChecklistMarkdown(index)
	if err := writeFile(*mdOut, []byte(body)); err != nil {
		return err
	}
	fmt.Printf("Wrote low-confidence checklist to %s\n", *mdOut)
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
