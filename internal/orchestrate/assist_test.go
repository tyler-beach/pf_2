package orchestrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tyler-beach/pf_2/internal/obligations"
)

func TestAssistOrchestratorNeverAutoApproves(t *testing.T) {
	root := t.TempDir()
	stmt := "Services should prefer async messaging where appropriate."
	body := "# ADR-001\n\n" + stmt + "\n"
	full := filepath.Join(root, "adrs/a.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	sha := hex.EncodeToString(sum[:])

	index := obligations.Index{
		SchemaVersion:     "1",
		SourceIndexSHA256: strings.Repeat("b", 64),
		Candidates: []obligations.Candidate{{
			ID: "CAND-1", ADRID: "ADR-001", Statement: stmt,
			Modality: "should", PatternHint: "manual_review", Kind: "normative_candidate",
			SourcePath: "adrs/a.md", SourceSHA256: sha,
			SourceLineStart: 3, SourceLineEnd: 3,
			ApprovalStatus: obligations.DecisionPending, EnforcementMode: obligations.EnforcementReviewRequired, Executable: false,
			Confidence: obligations.ConfidenceLow, ConfidenceP: 0.2, ConfidenceMethod: "platt_conformal",
			EscalationReasons: []string{"manual_review_pattern_hint"},
		}},
	}
	orch := NewDefaultAssistOrchestrator()
	out, err := orch.Assist(context.Background(), index, obligations.AssistOptions{
		MaxConfidence: obligations.ConfidenceLow,
		SourceRoot:    root,
		Provider:      obligations.OfflineAssistProvider{ModelID: "offline-test"},
		Now:           func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ProposalCount != 1 {
		t.Fatalf("proposals=%d", out.ProposalCount)
	}
	if out.Proposals[0].Executable || out.Proposals[0].ApprovalStatus != obligations.DecisionPending {
		t.Fatalf("HITL violated: %+v", out.Proposals[0])
	}
}

func TestAssertAssistHITLRejectsExecutable(t *testing.T) {
	err := assertAssistHITL(obligations.ProposalIndex{Proposals: []obligations.Proposal{{
		ProposalID: "PROP-1", Executable: true, ApprovalStatus: obligations.DecisionPending,
	}}})
	if err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("expected executable HITL error, got %v", err)
	}
}
