package obligations

import (
	"strings"
	"testing"
	"time"
)

func TestProposalToReviewAndAcceptRate(t *testing.T) {
	prop := Proposal{
		ProposalID: "PROP-1", ParentCandidateID: "CAND-1",
		SuggestedPatternID: "manual_review", Rationale: "from assist",
		ApprovalStatus: DecisionPending, Executable: false,
		SourceSHA256: strings.Repeat("a", 64), SourceLineStart: 3, SourceLineEnd: 3,
	}
	review, err := ProposalToReview(prop, DecisionApproved, "architect", "https://example/pr/1", "", EnforcementReviewRequired, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if review.CandidateID != "CAND-1" || review.PatternID != "manual_review" || review.Decision != DecisionApproved {
		t.Fatalf("unexpected review: %+v", review)
	}
	if _, err := ProposalToReview(prop, "pending_review", "a", "b", "c", "", time.Now()); err == nil {
		t.Fatal("expected invalid decision error")
	}

	index, catalog, cand := reviewFixture()
	prop.ParentCandidateID = cand.ID
	prop.SourceSHA256 = cand.SourceSHA256
	prop.SourceLineStart = cand.SourceLineStart
	prop.SourceLineEnd = cand.SourceLineEnd
	prop.SuggestedPatternID = "manual_review"
	proposals := ProposalIndex{Proposals: []Proposal{prop}, ProposalCount: 1}
	reg := NewRegistry()
	next, _, _, err := AcceptAssistProposal(index, catalog, reg, proposals, "PROP-1", DecisionApproved, "architect", "https://example/issues/1", "ok", EnforcementReviewRequired, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	rate, ok := AssistAcceptRate(proposals, next)
	if !ok || rate != 1 {
		t.Fatalf("accept rate = %v ok=%v", rate, ok)
	}

	lowOnly := Index{SourceRoot: "adrs", CandidateCount: 2, Candidates: []Candidate{
		{ID: "hi", Confidence: ConfidenceHigh, Statement: "must keep high"},
		{ID: "lo", Confidence: ConfidenceLow, Statement: "should escalate", PatternHint: "manual_review",
			SourcePath: "a.md", SourceLineStart: 1, SourceLineEnd: 1, EscalationReasons: []string{"manual_review_pattern_hint"}},
	}}
	md := LowConfidenceChecklistMarkdown(lowOnly)
	if !strings.Contains(md, "lo") || strings.Contains(md, "| hi |") {
		t.Fatalf("checklist should list only low candidates:\n%s", md)
	}
}
