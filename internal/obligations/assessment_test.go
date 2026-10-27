package obligations

import (
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

func repositoryEvidence(status string) assessment.Result {
	return assessment.Result{
		SchemaVersion: "1",
		EvidenceMode: string(assessment.EvidenceModeRepository),
		Verdict: "PASS",
		Requirements: []assessment.RequirementResult{{
			ADR: "ADR-001", ADRVersion: 1, RequirementID: "REQ-001-01",
			Title: "Domain code must not depend on infrastructure adapters",
			Status: status, Severity: "error", Evidence: []string{"repo snapshot sha256:fixture"},
		}},
	}
}

func TestAssessCandidatesKeepsUnreviewedCandidatesIndeterminate(t *testing.T) {
	index, catalog, _ := reviewFixture()
	report, err := AssessCandidates(index, catalog, NewRegistry(), repositoryEvidence("PASS"))
	if err != nil { t.Fatal(err) }
	if report.CandidateCount != 1 || report.Counts["INDETERMINATE"] != 1 || report.Counts["PASS"] != 0 {
		t.Fatalf("unreviewed candidate was treated as compliant: %+v", report)
	}
	item := report.Candidates[0]
	if item.Status != "INDETERMINATE" || item.EvidenceStatus != "MISSING" || !strings.Contains(item.Reason, "not been reviewed") {
		t.Fatalf("unexpected unreviewed candidate result: %+v", item)
	}
}

func TestAssessCandidatesMapsApprovedBindingToEvidenceStatus(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	registry, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil { t.Fatal(err) }
	for _, tc := range []struct{ evidence, want string }{
		{"PASS", "PASS"}, {"FAIL", "FAIL"}, {"INDETERMINATE", "INDETERMINATE"}, {"UNSUPPORTED", "INDETERMINATE"},
	} {
		t.Run(tc.evidence, func(t *testing.T) {
			report, err := AssessCandidates(index, catalog, registry, repositoryEvidence(tc.evidence))
			if err != nil { t.Fatal(err) }
			item := report.Candidates[0]
			if item.Status != tc.want || item.EvidenceStatus != tc.evidence {
				t.Fatalf("evidence status %s mapped to %+v; want %s", tc.evidence, item, tc.want)
			}
			if tc.evidence == "PASS" && len(item.Evidence) != 1 {
				t.Fatalf("assessment evidence was not preserved: %+v", item)
			}
		})
	}
}

func TestAssessCandidatesNeverPassesReviewRequiredOrMissingEvidence(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	proposal := validReviewProposal(candidate)
	proposal.EnforcementMode = EnforcementReviewRequired
	registry, _, err := RecordReview(index, catalog, NewRegistry(), proposal)
	if err != nil { t.Fatal(err) }
	report, err := AssessCandidates(index, catalog, registry, repositoryEvidence("PASS"))
	if err != nil { t.Fatal(err) }
	if report.Candidates[0].Status != "INDETERMINATE" || !strings.Contains(report.Candidates[0].Reason, "review_required") {
		t.Fatalf("review-required binding incorrectly passed: %+v", report.Candidates[0])
	}

	proposal = validReviewProposal(candidate)
	registry, _, err = RecordReview(index, catalog, NewRegistry(), proposal)
	if err != nil { t.Fatal(err) }
	missing := repositoryEvidence("PASS")
	missing.Requirements = nil
	report, err = AssessCandidates(index, catalog, registry, missing)
	if err != nil { t.Fatal(err) }
	if report.Candidates[0].Status != "INDETERMINATE" || report.Candidates[0].EvidenceStatus != "MISSING" {
		t.Fatalf("missing requirement evidence incorrectly passed: %+v", report.Candidates[0])
	}
}

func TestAssessCandidatesRejectsSubmissionModeAndDuplicateEvidence(t *testing.T) {
	index, catalog, _ := reviewFixture()
	submission := repositoryEvidence("PASS")
	submission.EvidenceMode = string(assessment.EvidenceModeSubmission)
	if _, err := AssessCandidates(index, catalog, NewRegistry(), submission); err == nil || !strings.Contains(err.Error(), "repository-mode") {
		t.Fatalf("submission evidence should not be accepted as repository evidence, got %v", err)
	}
	duplicate := repositoryEvidence("PASS")
	duplicate.Requirements = append(duplicate.Requirements, duplicate.Requirements[0])
	if _, err := AssessCandidates(index, catalog, NewRegistry(), duplicate); err == nil || !strings.Contains(err.Error(), "duplicate requirement result") {
		t.Fatalf("duplicate evidence IDs should be rejected, got %v", err)
	}
}
