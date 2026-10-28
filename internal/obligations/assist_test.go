package obligations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeAssistSource(t *testing.T, root, rel, body string) (sha string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func TestAssistObligationsOfflineNeverExecutable(t *testing.T) {
	root := t.TempDir()
	stmt := "Services should prefer async messaging where appropriate."
	body := "# ADR-001\n\n" + stmt + "\n"
	sha := writeAssistSource(t, root, "adrs/a.md", body)
	_ = writeAssistSource(t, root, "adrs/b.md", "# ADR-002\n\nServices must set a timeout.\n")

	index := Index{
		SchemaVersion:     "1",
		SourceIndexSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Candidates: []Candidate{{
			ID: "CAND-1", ADRID: "ADR-001", Statement: stmt,
			Modality: "should", PatternHint: "manual_review", Kind: "normative_candidate",
			SourcePath: "adrs/a.md", SourceSHA256: sha,
			SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
			Confidence: ConfidenceLow, ConfidenceP: 0.2, ConfidenceMethod: "platt_conformal", EscalationReasons: []string{"manual_review_pattern_hint"},
		}, {
			ID: "CAND-2", ADRID: "ADR-002", Statement: "Services must set a timeout.",
			Modality: "must", PatternHint: "configuration.bounds_or_required_values", Kind: "normative_candidate",
			SourcePath: "adrs/b.md", SourceSHA256: strings.Repeat("a", 64),
			SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
			Confidence: ConfidenceHigh, ConfidenceP: 0.9, ConfidenceMethod: "platt_conformal",
		}},
	}
	out, err := AssistObligations(context.Background(), index, AssistOptions{
		MaxConfidence: ConfidenceLow,
		SourceRoot:    root,
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
	if p.ParentCandidateID != "CAND-1" || p.SourceQuote != stmt {
		t.Fatalf("citation mismatch: %+v", p)
	}
	raw, _ := json.Marshal(out)
	if string(raw) == "" {
		t.Fatal("empty marshal")
	}
}

func TestVerifyAssistCitationRejectsSHADriftAndBadLines(t *testing.T) {
	root := t.TempDir()
	stmt := "Services must set a finite timeout."
	body := "# Title\n\n" + stmt + "\n"
	sha := writeAssistSource(t, root, "adrs/x.md", body)
	cand := Candidate{
		ID: "CAND-X", Statement: stmt, SourcePath: "adrs/x.md", SourceSHA256: sha,
		SourceLineStart: 3, SourceLineEnd: 3,
	}
	if err := verifyAssistCitation(root, cand, stmt); err != nil {
		t.Fatalf("expected valid citation: %v", err)
	}
	cand.SourceSHA256 = strings.Repeat("0", 64)
	if err := verifyAssistCitation(root, cand, stmt); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("expected SHA mismatch, got %v", err)
	}
	cand.SourceSHA256 = sha
	cand.SourceLineStart, cand.SourceLineEnd = 1, 1
	if err := verifyAssistCitation(root, cand, stmt); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected line-range miss, got %v", err)
	}
	if err := verifyAssistCitation("", cand, stmt); err == nil {
		t.Fatal("expected missing root error")
	}
}

type stubAssistProvider struct {
	usage TokenUsage
	raw   []byte
}

func (s stubAssistProvider) Name() string  { return "stub" }
func (s stubAssistProvider) Model() string { return "stub-model" }
func (s stubAssistProvider) Complete(ctx context.Context, prompt string) ([]byte, TokenUsage, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, TokenUsage{}, ctx.Err()
	}
	return append([]byte(nil), s.raw...), s.usage, nil
}

func TestAssistObligationsAccumulatesProviderTokenUsage(t *testing.T) {
	root := t.TempDir()
	stmt := "Services should prefer async messaging where appropriate."
	body := "# ADR-001\n\n" + stmt + "\n"
	sha := writeAssistSource(t, root, "adrs/a.md", body)
	raw, _ := json.Marshal(map[string]string{
		"normalized_statement": "prefer async",
		"modality":             "should",
		"suggested_pattern_id": "manual_review",
		"rationale":            "test stub",
	})
	index := Index{
		SchemaVersion: "1", SourceIndexSHA256: strings.Repeat("b", 64),
		Candidates: []Candidate{{
			ID: "CAND-1", ADRID: "ADR-001", Statement: stmt,
			Modality: "should", PatternHint: "manual_review", Kind: "normative_candidate",
			SourcePath: "adrs/a.md", SourceSHA256: sha, SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
			Confidence: ConfidenceLow,
		}},
	}
	out, err := AssistObligations(context.Background(), index, AssistOptions{
		MaxConfidence: ConfidenceLow, SourceRoot: root,
		Provider: stubAssistProvider{usage: TokenUsage{PromptTokens: 11, CompletionTokens: 7}, raw: raw},
		Now:      func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.PromptTokens != 11 || out.CompletionTokens != 7 {
		t.Fatalf("token usage not accumulated: prompt=%d completion=%d", out.PromptTokens, out.CompletionTokens)
	}
}

func TestAssistObligationsRequiresSourceRoot(t *testing.T) {
	index := Index{Candidates: []Candidate{{
		ID: "CAND-1", Statement: "x", Modality: "should", PatternHint: "manual_review",
		Kind: "normative_candidate", SourcePath: "a.md", SourceSHA256: strings.Repeat("a", 64),
		SourceLineStart: 1, SourceLineEnd: 1, Confidence: ConfidenceLow, Executable: false,
		ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired,
	}}}
	_, err := AssistObligations(context.Background(), index, AssistOptions{
		MaxConfidence: ConfidenceLow,
		Provider:      OfflineAssistProvider{},
	})
	if err == nil || !strings.Contains(err.Error(), "source root") {
		t.Fatalf("expected source root error, got %v", err)
	}
}
