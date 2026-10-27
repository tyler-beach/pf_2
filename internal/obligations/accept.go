package obligations

import (
	"fmt"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

// LowConfidenceChecklistMarkdown renders an actionable review checklist for
// low-confidence candidates only. It never marks candidates approved.
func LowConfidenceChecklistMarkdown(index Index) string {
	var b strings.Builder
	b.WriteString("# Low-confidence obligation checklist\n\n")
	b.WriteString("> Escalation queue only: these candidates are **not** approvals and are **not** executable.\n")
	b.WriteString("> Use `adrctl assist-obligations` for batch proposals, then `adrctl accept-assist-proposal` after human review.\n\n")
	low := FilterByMaxConfidence(index, ConfidenceLow)
	fmt.Fprintf(&b, "- Source root: `%s`\n", mdCell(index.SourceRoot))
	fmt.Fprintf(&b, "- Low-confidence candidates: %d of %d\n\n", len(low), index.CandidateCount)
	if len(low) == 0 {
		b.WriteString("No low-confidence candidates. Nothing to escalate.\n")
		return b.String()
	}
	b.WriteString("| Done | Candidate | Source | Pattern hint | Escalation reasons | Statement |\n|---|---|---|---|---|---|\n")
	for _, candidate := range low {
		fmt.Fprintf(&b, "| [ ] | %s | %s:%d-%d | %s | %s | %s |\n",
			mdCell(candidate.ID),
			mdCell(candidate.SourcePath), candidate.SourceLineStart, candidate.SourceLineEnd,
			mdCell(candidate.PatternHint),
			mdCell(strings.Join(candidate.EscalationReasons, ", ")),
			mdCell(candidate.Statement))
	}
	b.WriteString("\n## Reviewer actions\n\n")
	b.WriteString("1. Confirm the source citation (path, SHA, line range) still matches the ADR.\n")
	b.WriteString("2. Decide whether the statement is normative; reject noise.\n")
	b.WriteString("3. If accepting an assist proposal, bind only via `accept-assist-proposal` / `record-obligation-review` — never auto-mutate the catalog.\n")
	b.WriteString("4. Prefer `manual_review` + `review_required` until a catalog requirement exists.\n")
	return b.String()
}

// ProposalToReview maps an assist proposal into a ReviewProposal for RecordReview.
// It never marks executable and never mutates the requirements catalog by itself.
func ProposalToReview(proposal Proposal, decision, reviewer, reviewReference, rationale, enforcementMode string, now time.Time) (ReviewProposal, error) {
	if proposal.Executable {
		return ReviewProposal{}, fmt.Errorf("refusing executable proposal %s", proposal.ProposalID)
	}
	if proposal.ApprovalStatus != "" && proposal.ApprovalStatus != DecisionPending {
		return ReviewProposal{}, fmt.Errorf("proposal %s is not pending_review", proposal.ProposalID)
	}
	parent := strings.TrimSpace(proposal.ParentCandidateID)
	if parent == "" {
		return ReviewProposal{}, fmt.Errorf("proposal %s missing parent_candidate_id", proposal.ProposalID)
	}
	decision = strings.TrimSpace(decision)
	switch decision {
	case DecisionApproved, DecisionRejected:
	default:
		return ReviewProposal{}, fmt.Errorf("decision must be approved or rejected, got %q", decision)
	}
	if strings.TrimSpace(reviewer) == "" {
		return ReviewProposal{}, fmt.Errorf("reviewer is required")
	}
	if strings.TrimSpace(reviewReference) == "" {
		return ReviewProposal{}, fmt.Errorf("review_reference is required")
	}
	if strings.TrimSpace(rationale) == "" {
		rationale = strings.TrimSpace(proposal.Rationale)
	}
	if rationale == "" {
		return ReviewProposal{}, fmt.Errorf("rationale is required")
	}
	mode := strings.TrimSpace(enforcementMode)
	if mode == "" {
		mode = EnforcementReviewRequired
	}
	pattern := strings.TrimSpace(proposal.SuggestedPatternID)
	if decision == DecisionRejected {
		pattern = ""
	}
	reviewedAt := now.UTC().Format(time.RFC3339)
	return ReviewProposal{
		CandidateID:     parent,
		Decision:        decision,
		PatternID:       pattern,
		EnforcementMode: mode,
		Reviewer:        strings.TrimSpace(reviewer),
		ReviewedAt:      reviewedAt,
		ReviewReference: strings.TrimSpace(reviewReference),
		Rationale:       rationale,
	}, nil
}

// AcceptAssistProposal records a human decision derived from an assist proposal.
func AcceptAssistProposal(index Index, catalog assessment.Catalog, registry Registry, proposals ProposalIndex, proposalID, decision, reviewer, reviewReference, rationale, enforcementMode string, now time.Time) (Registry, BindingCoverage, Proposal, error) {
	var found Proposal
	ok := false
	for _, p := range proposals.Proposals {
		if p.ProposalID == proposalID {
			found = p
			ok = true
			break
		}
	}
	if !ok {
		return registry, BindingCoverage{}, Proposal{}, fmt.Errorf("proposal %s not found", proposalID)
	}
	parent, present, err := findCandidate(index, found.ParentCandidateID)
	if err != nil {
		return registry, BindingCoverage{}, Proposal{}, err
	}
	if !present {
		return registry, BindingCoverage{}, Proposal{}, fmt.Errorf("parent candidate %s missing from index", found.ParentCandidateID)
	}
	if parent.SourceSHA256 != found.SourceSHA256 || parent.SourceLineStart != found.SourceLineStart || parent.SourceLineEnd != found.SourceLineEnd {
		return registry, BindingCoverage{}, Proposal{}, fmt.Errorf("proposal %s source citation drifted from candidate %s", proposalID, parent.ID)
	}
	review, err := ProposalToReview(found, decision, reviewer, reviewReference, rationale, enforcementMode, now)
	if err != nil {
		return registry, BindingCoverage{}, Proposal{}, err
	}
	next, coverage, err := RecordReview(index, catalog, registry, review)
	if err != nil {
		return registry, BindingCoverage{}, Proposal{}, err
	}
	return next, coverage, found, nil
}

// AssistAcceptRate computes approved/(approved+rejected) among proposal parents
// that have a non-pending latest review. Returns ok=false when denominator is 0.
func AssistAcceptRate(proposals ProposalIndex, registry Registry) (rate float64, ok bool) {
	latest := latestRecords(registry)
	approved, rejected := 0, 0
	seen := map[string]bool{}
	for _, p := range proposals.Proposals {
		parent := strings.TrimSpace(p.ParentCandidateID)
		if parent == "" || seen[parent] {
			continue
		}
		seen[parent] = true
		rec, present := latest[parent]
		if !present {
			continue
		}
		switch rec.Decision {
		case DecisionApproved:
			approved++
		case DecisionRejected:
			rejected++
		}
	}
	denom := approved + rejected
	if denom == 0 {
		return 0, false
	}
	return float64(approved) / float64(denom), true
}
