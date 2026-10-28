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

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
	"github.com/tyler-beach/pf_2/internal/orchestrate"
)

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
	// AssistOrchestrator is the sole LLM seam: proposals stay pending_review /
	// non-executable. Approval is a separate human review command.
	assistOrch := orchestrate.DefaultAssistOrchestrator{Observer: cliObserver()}
	out, err := assistOrch.Assist(ctx, index, obligations.AssistOptions{
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
