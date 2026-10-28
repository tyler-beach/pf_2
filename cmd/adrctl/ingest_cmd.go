package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
	"github.com/tyler-beach/pf_2/internal/orchestrate"
)

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

func runExtractObligations(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/ingested-adrs.json", "deterministic ADR ingestion index JSON")
	root := fs.String("root", "", "directory containing source ADR Markdown; defaults to source_root from index")
	output := fs.String("json-out", "adr/obligation-candidates.json", "review-only candidate obligation JSON output")
	reportPath := fs.String("report", "", "optional Markdown review queue output")
	minConfidenceReport := fs.Bool("min-confidence-report", false, "print confidence-band summary to stdout")
	extractLLM := fs.Bool("extract-llm", false, "optional batch LLM scaffolding for tech_spec/mixed prose (off by default; or PATHFINDER_EXTRACT_LLM=1)")
	extractLLMForce := fs.Bool("extract-llm-force", false, "bypass submission_kind gate for extract-llm (still pending_review only)")
	submissionKind := fs.String("submission-kind", "tech_spec", "input shape for extract-llm gate: tech_spec|mixed|code_snippet")
	providerName := fs.String("extract-llm-provider", "offline", "extract-llm provider: offline|openai-compatible")
	model := fs.String("extract-llm-model", "", "model id for live extract-llm (optional label for offline)")
	endpoint := fs.String("extract-llm-endpoint", "", "OpenAI-compatible chat completions endpoint")
	apiKeyEnv := fs.String("extract-llm-api-key-env", "LLM_API_KEY", "env var holding API key for live extract-llm")
	maxTokens := fs.Int("extract-llm-max-tokens", 512, "spend guard: max tokens per extract-llm completion")
	maxCandidates := fs.Int("extract-llm-max-candidates", 32, "spend guard: max LLM-proposed candidates")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("extract-obligations accepts no positional arguments")
	}
	enableLLM := *extractLLM || obligations.ExtractLLMEnabledFromEnv()
	kind, err := assessment.ParseSubmissionKind(*submissionKind)
	if err != nil {
		return fmt.Errorf("submission-kind: %w", err)
	}
	if enableLLM && !*extractLLMForce && !kind.AllowsLLMExtract() {
		return fmt.Errorf("extract-llm refused for submission_kind %q (allowed: tech_spec, mixed); use --extract-llm-force only with explicit operator intent", kind)
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
	var provider obligations.AssistProvider
	if enableLLM {
		switch strings.ToLower(strings.TrimSpace(*providerName)) {
		case "offline", "offline-fixture":
			provider = obligations.OfflineExtractLLMProvider{ModelID: *model}
		case "openai-compatible":
			apiKey := strings.TrimSpace(os.Getenv(*apiKeyEnv))
			if apiKey == "" {
				return fmt.Errorf("live extract-llm requires API key in $%s", *apiKeyEnv)
			}
			if strings.TrimSpace(*model) == "" {
				return fmt.Errorf("--extract-llm-model is required for openai-compatible provider")
			}
			provider = obligations.OpenAICompatProvider{
				Endpoint:  *endpoint,
				APIKey:    apiKey,
				ModelID:   *model,
				MaxTokens: *maxTokens,
			}
		default:
			return fmt.Errorf("unsupported extract-llm provider %q (want offline|openai-compatible)", *providerName)
		}
	}
	kindLabel := string(kind)
	if kindLabel == "" {
		kindLabel = string(assessment.SubmissionKindTechSpec)
	}
	orch := orchestrate.DefaultExtractLLMOrchestrator{Observer: cliObserver()}
	candidates, err := orch.Build(ctx, index.Documents, *root, index.SourceRoot, indexBytes, enableLLM, obligations.ExtractLLMOptions{
		SubmissionKind: kindLabel,
		Force:          *extractLLMForce,
		Provider:       provider,
		MaxCandidates:  *maxCandidates,
	})
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
	if enableLLM {
		fmt.Printf("; extract-llm scaffolding llm_candidates=%d runtime_llm_calls=%d (pending_review only)",
			candidates.LLMExtract.LLMCandidateCount, candidates.LLMExtract.RuntimeLLMCalls)
	}
	fmt.Printf(". No candidate was approved or made enforceable.\n")
	if *minConfidenceReport {
		fmt.Printf("Confidence summary: high=%d medium=%d low=%d (low band is eligible for batch LLM assist)\n",
			summary.High, summary.Medium, summary.Low)
	}
	return nil
}
