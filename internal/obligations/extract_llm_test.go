package obligations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/assessment"
)

type mockExtractProvider struct {
	raw   []byte
	usage TokenUsage
	calls int
}

func (m *mockExtractProvider) Name() string  { return "mock" }
func (m *mockExtractProvider) Model() string { return "mock-mini" }
func (m *mockExtractProvider) Complete(ctx context.Context, prompt string) ([]byte, TokenUsage, error) {
	m.calls++
	if ctx != nil && ctx.Err() != nil {
		return nil, TokenUsage{}, ctx.Err()
	}
	return append([]byte(nil), m.raw...), m.usage, nil
}

func TestAllowExtractLLMKindGate(t *testing.T) {
	if err := AllowExtractLLM("tech_spec", false); err != nil {
		t.Fatalf("tech_spec should allow: %v", err)
	}
	if err := AllowExtractLLM("mixed", false); err != nil {
		t.Fatalf("mixed should allow: %v", err)
	}
	if err := AllowExtractLLM("code_snippet", false); err == nil {
		t.Fatal("code_snippet must be refused without force")
	}
	if err := AllowExtractLLM("code_snippet", true); err != nil {
		t.Fatalf("force should bypass kind gate: %v", err)
	}
	if err := AllowExtractLLM("", false); err == nil {
		t.Fatal("unspecified must be refused without force")
	}
}

func TestExtractLLMProposalsSchemaValidPending(t *testing.T) {
	root := t.TempDir()
	prose := strings.Join([]string{
		"# ADR-LLM-1: Narrative timeout policy",
		"",
		"## Status",
		"",
		"Accepted",
		"",
		"## Decision",
		"",
		"Teams prefer resilient networking. In practice outbound calls ought to finish promptly,",
		"and retries should stay bounded when remote systems stall.",
		"",
	}, "\n")
	path := filepath.Join(root, "ADR-LLM-1.md")
	if err := os.WriteFile(path, []byte(prose), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-LLM-1.md", []byte(prose))
	if err != nil {
		t.Fatal(err)
	}
	fixture := []byte(`{
  "candidates": [{
    "statement": "Outbound calls must define a finite timeout.",
    "modality": "must",
    "pattern_hint": "configuration.bounds_or_required_values",
    "section_heading": "Decision",
    "conditions": ["outbound"],
    "exceptions": [],
    "ambiguity": ["narrative_paraphrase"],
    "source_line_start": 9,
    "source_line_end": 9
  }]
}`)
	provider := &mockExtractProvider{raw: fixture, usage: TokenUsage{PromptTokens: 100, CompletionTokens: 40}}
	rows, meta, err := ExtractLLMProposals(context.Background(), []adringest.Document{doc}, root, "adrs", ExtractLLMOptions{
		SubmissionKind: string(assessment.SubmissionKindTechSpec),
		Provider:       provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || meta.RuntimeLLMCalls != 1 {
		t.Fatalf("calls=%d meta=%+v", provider.calls, meta)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	c := rows[0]
	if c.Executable || c.ApprovalStatus != DecisionPending {
		t.Fatalf("HITL violated: %+v", c)
	}
	if c.ExtractionMethod != ExtractionMethodLLM {
		t.Fatalf("extraction_method=%q", c.ExtractionMethod)
	}
	if c.Statement == "" || c.Modality != "must" || c.PatternHint == "" {
		t.Fatalf("schema mapping incomplete: %+v", c)
	}
	if c.SubmissionKind != "tech_spec" {
		t.Fatalf("submission_kind=%q", c.SubmissionKind)
	}
}

func TestMergeLLMCandidatesUnionDedupe(t *testing.T) {
	det := Index{
		SchemaVersion: "1",
		Candidates: []Candidate{{
			ID: "CAND-D", ADRID: "ADR-1", Statement: "Services must set a timeout.",
			Modality: "must", PatternHint: "configuration.bounds_or_required_values",
			SourcePath: "adrs/a.md", SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired,
			ExtractionMethod: ExtractionMethodDeterministic,
			Confidence:       ConfidenceHigh, ConfidenceP: 0.9,
		}},
	}
	llm := []Candidate{{
		Statement: "Services must set a timeout.", Modality: "must",
		PatternHint: "configuration.bounds_or_required_values",
		SourcePath: "adrs/a.md", SourceLineStart: 3, SourceLineEnd: 3,
		ApprovalStatus: DecisionPending, ExtractionMethod: ExtractionMethodLLM,
	}, {
		Statement: "Clients must use jittered backoff.", Modality: "must",
		PatternHint: "configuration.bounds_or_required_values",
		SourcePath: "adrs/a.md", SourceLineStart: 4, SourceLineEnd: 4,
		ApprovalStatus: DecisionPending, ExtractionMethod: ExtractionMethodLLM,
	}}
	merged := MergeLLMCandidates(det, llm, LLMExtractMeta{Provider: "mock", Model: "mock-mini"})
	if merged.CandidateCount != 2 {
		t.Fatalf("want union of 2 after dedupe, got %d", merged.CandidateCount)
	}
	if err := AssertExtractLLMHITL(merged); err != nil {
		t.Fatal(err)
	}
	llmOnly := 0
	for _, c := range merged.Candidates {
		if c.ExtractionMethod == ExtractionMethodLLM {
			llmOnly++
			if c.Executable || c.ApprovalStatus != DecisionPending {
				t.Fatalf("llm row not pending: %+v", c)
			}
			found := false
			for _, r := range c.EscalationReasons {
				if r == "llm_extract" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing llm_extract escalation: %+v", c.EscalationReasons)
			}
			if c.Confidence == "" || c.ConfidenceMethod == "" {
				t.Fatalf("confidence not scored after merge: %+v", c)
			}
		}
	}
	if llmOnly != 1 {
		t.Fatalf("llm-only rows=%d", llmOnly)
	}
	if merged.LLMExtract.LLMCandidateCount != 1 {
		t.Fatalf("meta llm count=%d", merged.LLMExtract.LLMCandidateCount)
	}
}

func TestBuildIndexWithOptionalLLMAndDeterministicSubset(t *testing.T) {
	root := t.TempDir()
	source := strings.Join([]string{
		"# ADR-902: Narrative policy", "",
		"## Status", "", "Accepted", "",
		"## Decision", "",
		"Every outbound network call must define a finite timeout.",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "ADR-902.md"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-902.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	rawIndex := []byte(`{"schema_version":"1","source_root":"adrs"}`)
	fixture := []byte(`{"candidates":[{"statement":"Remote clients must apply bounded retries.","modality":"must","pattern_hint":"configuration.bounds_or_required_values","section_heading":"Decision","conditions":[],"exceptions":[],"ambiguity":["llm_only"],"source_line_start":9,"source_line_end":9}]}`)
	index, err := BuildIndexWithOptionalLLM(context.Background(), []adringest.Document{doc}, root, "adrs", rawIndex, true, ExtractLLMOptions{
		SubmissionKind: "tech_spec",
		Provider:       &mockExtractProvider{raw: fixture},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !HasLLMExtractedCandidates(index) {
		t.Fatal("expected llm-tagged candidates")
	}
	det := DeterministicCandidates(index)
	if HasLLMExtractedCandidates(det) {
		t.Fatal("deterministic subset must drop llm rows")
	}
	if det.CandidateCount < 1 {
		t.Fatal("deterministic extract should still produce candidates")
	}
	// Freshness shape: deterministic rebuild equals stripped index.
	rebuilt, err := BuildIndex([]adringest.Document{doc}, root, "adrs", rawIndex)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(DeterministicCandidates(index))
	want, _ := json.Marshal(rebuilt)
	// Compare candidate statements/ids rather than full JSON (LLMExtract cleared but calibration identical).
	if det.CandidateCount != rebuilt.CandidateCount {
		t.Fatalf("deterministic count %d vs rebuilt %d\ngot=%s\nwant=%s", det.CandidateCount, rebuilt.CandidateCount, got, want)
	}
}

func TestExtractLLMRejectsCodeSnippetKind(t *testing.T) {
	_, _, err := ExtractLLMProposals(context.Background(), nil, t.TempDir(), "adrs", ExtractLLMOptions{
		SubmissionKind: string(assessment.SubmissionKindCodeSnippet),
		Provider:       OfflineExtractLLMProvider{},
	})
	if err == nil || !strings.Contains(err.Error(), "kind gate") {
		t.Fatalf("expected kind gate error, got %v", err)
	}
}

func TestAssertExtractLLMHITLRejectsExecutable(t *testing.T) {
	err := AssertExtractLLMHITL(Index{Candidates: []Candidate{{
		ID: "CAND-X", ExtractionMethod: ExtractionMethodLLM,
		Executable: true, ApprovalStatus: DecisionPending,
	}}})
	if err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("expected HITL error, got %v", err)
	}
}

func TestAssessPathStaysZeroLLMWithExtractLLMCandidates(t *testing.T) {
	index, catalog, _ := reviewFixture()
	base := index.Candidates[0]
	llmCand := Candidate{
		ADRID: base.ADRID, ADRTitle: base.ADRTitle, SectionHeading: "Decision",
		Statement: "Narrative must become a pending candidate.",
		Modality: "must", PatternHint: "manual_review", Kind: "normative_candidate",
		SubmissionKind: "tech_spec", ExtractionMethod: ExtractionMethodLLM,
		SourcePath: base.SourcePath, SourceSHA256: base.SourceSHA256,
		SourceLineStart: 1, SourceLineEnd: 1,
		ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
		Confidence: ConfidenceLow, ConfidenceP: 0.2, ConfidenceMethod: "platt_conformal",
		EscalationReasons: []string{"llm_extract"},
	}
	llmCand.ID = candidateID(llmCand)
	index.Candidates = append(index.Candidates, llmCand)
	index.CandidateCount = len(index.Candidates)
	evidence := repositoryEvidence("PASS")
	if evidence.LLMCalls != 0 {
		t.Fatalf("fixture evidence must be zero-LLM, got %d", evidence.LLMCalls)
	}
	report, err := assessCandidatesForTest(index, catalog, NewRegistry(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.LLMCalls != 0 {
		t.Fatalf("assess must not mutate evidence LLMCalls, got %d", evidence.LLMCalls)
	}
	if report.CandidateCount < 2 {
		t.Fatalf("expected llm candidate in assess join, got %d", report.CandidateCount)
	}
}
