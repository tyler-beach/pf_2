package orchestrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

func TestExtractLLMOrchestratorHITL(t *testing.T) {
	root := t.TempDir()
	source := "# ADR-E1: Spec\n\n## Decision\n\nServices must set a timeout.\n"
	if err := os.WriteFile(filepath.Join(root, "ADR-E1.md"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-E1.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	orch := NewDefaultExtractLLMOrchestrator()
	fixture := []byte(`{"candidates":[{"statement":"Caches must expire secrets within 24 hours.","modality":"must","pattern_hint":"manual_review","section_heading":"Decision","conditions":[],"exceptions":[],"ambiguity":[],"source_line_start":1,"source_line_end":1}]}`)
	out, err := orch.Build(context.Background(), []adringest.Document{doc}, root, "adrs",
		[]byte(`{"schema_version":"1","source_root":"adrs"}`), true, obligations.ExtractLLMOptions{
			SubmissionKind: "tech_spec",
			Provider:       obligations.OfflineExtractLLMProvider{Fixture: fixture},
		})
	if err != nil {
		t.Fatal(err)
	}
	if !obligations.HasLLMExtractedCandidates(out) {
		t.Fatal("expected llm candidates")
	}
	for _, c := range out.Candidates {
		if c.Executable || (c.ApprovalStatus != "" && c.ApprovalStatus != obligations.DecisionPending) {
			t.Fatalf("HITL violated: %+v", c)
		}
	}
}

func TestAssertExtractLLMHITLOrchestrate(t *testing.T) {
	err := assertExtractLLMHITL(obligations.Index{Candidates: []obligations.Candidate{{
		ID: "CAND-1", ExtractionMethod: obligations.ExtractionMethodLLM,
		Executable: true, ApprovalStatus: obligations.DecisionPending,
	}}})
	if err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("expected executable HITL error, got %v", err)
	}
}
