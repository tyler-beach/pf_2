package obligations

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestAssistObligationsOfflineNeverExecutable(t *testing.T) {
	index := Index{
		SchemaVersion:     "1",
		SourceIndexSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Candidates: []Candidate{{
			ID: "CAND-1", ADRID: "ADR-001", Statement: "Services should prefer async messaging where appropriate.",
			Modality: "should", PatternHint: "manual_review", Kind: "normative_candidate",
			SourcePath: "adrs/a.md", SourceSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
			Confidence: ConfidenceLow, ConfidenceScore: 0.2, EscalationReasons: []string{"manual_review_pattern_hint"},
		}, {
			ID: "CAND-2", ADRID: "ADR-002", Statement: "Services must set a timeout.",
			Modality: "must", PatternHint: "configuration.bounds_or_required_values", Kind: "normative_candidate",
			SourcePath: "adrs/b.md", SourceSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
			Confidence: ConfidenceHigh, ConfidenceScore: 0.9,
		}},
	}
	out, err := AssistObligations(context.Background(), index, AssistOptions{
		MaxConfidence: ConfidenceLow,
		Provider:      OfflineAssistProvider{ModelID: "offline-test"},
		Now:           func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ProposalCount != 1 || out.RuntimeLLMCalls != 1 {
		t.Fatalf("unexpected assist result: %+v", out)
	}
	p := out.Proposals[0]
	if p.Executable || p.ApprovalStatus != DecisionPending {
		t.Fatalf("proposal must stay review-only: %+v", p)
	}
	if p.ParentCandidateID != "CAND-1" || p.SourceQuote != index.Candidates[0].Statement {
		t.Fatalf("citation mismatch: %+v", p)
	}
	raw, _ := json.Marshal(out)
	if string(raw) == "" {
		t.Fatal("empty marshal")
	}
}
