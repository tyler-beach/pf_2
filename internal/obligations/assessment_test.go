package obligations

import (
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
)

func repositoryEvidence(status string) assessment.Result {
	return assessment.Result{
		SchemaVersion:        "1",
		EvidenceMode:         string(assessment.EvidenceModeRepository),
		Verdict:              "PASS",
		SourceSnapshotSHA256: strings.Repeat("a", 64),
		Requirements: []assessment.RequirementResult{{
			ADR: "ADR-001", ADRVersion: 1, RequirementID: "REQ-001-01",
			Title:  "Domain code must not depend on infrastructure adapters",
			Status: status, Severity: "error", Evidence: []string{"repo snapshot sha256:fixture"},
		}},
	}
}

func assessCandidatesForTest(index Index, catalog assessment.Catalog, registry Registry, evidence assessment.Result) (AssessmentReport, error) {
	return AssessCandidates(index, catalog, registry, evidence, evidence.SourceSnapshotSHA256, strings.Repeat("b", 64))
}

func TestAssessCandidatesKeepsUnreviewedCandidatesIndeterminate(t *testing.T) {
	index, catalog, _ := reviewFixture()
	report, err := assessCandidatesForTest(index, catalog, NewRegistry(), repositoryEvidence("PASS"))
	if err != nil {
		t.Fatal(err)
	}
	if report.CandidateCount != 1 || report.Counts["INDETERMINATE"] != 1 || report.Counts["PASS"] != 0 || report.ObligationVerdict != "INDETERMINATE" {
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
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ evidence, want string }{
		{"PASS", "PASS"}, {"FAIL", "FAIL"}, {"INDETERMINATE", "INDETERMINATE"}, {"UNSUPPORTED", "INDETERMINATE"},
	} {
		t.Run(tc.evidence, func(t *testing.T) {
			report, err := assessCandidatesForTest(index, catalog, registry, repositoryEvidence(tc.evidence))
			if err != nil {
				t.Fatal(err)
			}
			item := report.Candidates[0]
			if item.Status != tc.want || item.EvidenceStatus != tc.evidence || report.ObligationVerdict != tc.want {
				t.Fatalf("evidence status %s mapped to %+v; want %s", tc.evidence, item, tc.want)
			}
			if tc.evidence == "PASS" && len(item.Evidence) != 1 {
				t.Fatalf("assessment evidence was not preserved: %+v", item)
			}
			if !validSHA256(item.RequirementSHA256) || !strings.Contains(MarkdownAssessmentReport(report), item.RequirementSHA256) {
				t.Fatalf("assessment report did not expose the exact approved binding snapshot: %+v", item)
			}
		})
	}
}

func TestAssessCandidatesDowngradesChangedVerifierBinding(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	registry, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil {
		t.Fatal(err)
	}

	// Keep the requirement ID and verifier type but change the actual policy.
	catalog.ADRs[0].Requirements[0].Verifier.ForbiddenImportPrefixes = []string{"internal/infrastructure/", "internal/secrets/"}
	report, err := assessCandidatesForTest(index, catalog, registry, repositoryEvidence("PASS"))
	if err != nil {
		t.Fatal(err)
	}
	item := report.Candidates[0]
	if item.Status != "INDETERMINATE" || item.EvidenceStatus != "STALE" ||
		!strings.Contains(item.Reason, "binding snapshot is stale") || report.BindingCoverage.StaleCount != 1 {
		t.Fatalf("changed verifier config retained prior approval: %+v; coverage=%+v", item, report.BindingCoverage)
	}
	if report.ObligationVerdict != "INDETERMINATE" {
		t.Fatalf("changed verifier config should not pass: %+v", report)
	}
}

func TestAssessCandidatesNeverPassesReviewRequiredOrMissingEvidence(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	proposal := validReviewProposal(candidate)
	proposal.EnforcementMode = EnforcementReviewRequired
	registry, _, err := RecordReview(index, catalog, NewRegistry(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	report, err := assessCandidatesForTest(index, catalog, registry, repositoryEvidence("PASS"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates[0].Status != "INDETERMINATE" || !strings.Contains(report.Candidates[0].Reason, "review_required") {
		t.Fatalf("review-required binding incorrectly passed: %+v", report.Candidates[0])
	}

	proposal = validReviewProposal(candidate)
	registry, _, err = RecordReview(index, catalog, NewRegistry(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	missing := repositoryEvidence("PASS")
	missing.Requirements = nil
	report, err = assessCandidatesForTest(index, catalog, registry, missing)
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates[0].Status != "INDETERMINATE" || report.Candidates[0].EvidenceStatus != "MISSING" {
		t.Fatalf("missing requirement evidence incorrectly passed: %+v", report.Candidates[0])
	}
}

func TestAssessCandidatesReturnsNotApplicableWhenAllCandidatesAreRejected(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	proposal := validReviewProposal(candidate)
	proposal.Decision = DecisionRejected
	proposal.RequirementID = ""
	proposal.PatternID = ""
	proposal.EnforcementMode = EnforcementReviewRequired
	registry, _, err := RecordReview(index, catalog, NewRegistry(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	report, err := assessCandidatesForTest(index, catalog, registry, repositoryEvidence("PASS"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates[0].Status != "NOT_APPLICABLE" || report.ObligationVerdict != "NOT_APPLICABLE" {
		t.Fatalf("all-rejected candidate set should be NOT_APPLICABLE: %+v", report)
	}
}

func TestAssessCandidatesDoesNotPassWhenEvidenceIsEmpty(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	registry, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil {
		t.Fatal(err)
	}
	evidence := repositoryEvidence("PASS")
	evidence.Requirements[0].Evidence = nil
	report, err := assessCandidatesForTest(index, catalog, registry, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates[0].Status != "INDETERMINATE" || report.Candidates[0].EvidenceStatus != "MISSING" {
		t.Fatalf("a result without evidence was treated as compliant: %+v", report.Candidates[0])
	}
}

func TestAssessCandidatesFailsClosedOnStaleSnapshotsAndRuntimeLLM(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	registry, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil {
		t.Fatal(err)
	}
	evidence := repositoryEvidence("PASS")
	report, err := AssessCandidates(index, catalog, registry, evidence, strings.Repeat("c", 64), strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if report.EvidenceFresh || report.RepositoryVerdict != "INDETERMINATE" || report.ObligationVerdict != "INDETERMINATE" ||
		report.Candidates[0].Status != "INDETERMINATE" || report.Candidates[0].EvidenceStatus != "STALE" {
		t.Fatalf("stale source evidence was not downgraded: %+v", report)
	}

	evidence = repositoryEvidence("PASS")
	evidence.AnalyzerResults = []analyzer.ScanResult{{InputSHA256: strings.Repeat("c", 64)}}
	report, err = AssessCandidates(index, catalog, registry, evidence, evidence.SourceSnapshotSHA256, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if report.EvidenceFresh || report.Candidates[0].Status != "INDETERMINATE" || report.Candidates[0].EvidenceStatus != "STALE" {
		t.Fatalf("stale analyzer snapshot was not downgraded: %+v", report)
	}

	evidence = repositoryEvidence("PASS")
	evidence.LLMCalls = 1
	if _, err := assessCandidatesForTest(index, catalog, registry, evidence); err == nil || !strings.Contains(err.Error(), "runtime LLM call") {
		t.Fatalf("runtime LLM evidence should be rejected: %v", err)
	}
}

func TestAssessCandidatesRejectsSubmissionModeAndDuplicateEvidence(t *testing.T) {
	index, catalog, _ := reviewFixture()
	submission := repositoryEvidence("PASS")
	submission.EvidenceMode = string(assessment.EvidenceModeSubmission)
	if _, err := assessCandidatesForTest(index, catalog, NewRegistry(), submission); err == nil || !strings.Contains(err.Error(), "repository-mode") {
		t.Fatalf("submission evidence should not be accepted as repository evidence, got %v", err)
	}
	duplicate := repositoryEvidence("PASS")
	duplicate.Requirements = append(duplicate.Requirements, duplicate.Requirements[0])
	if _, err := assessCandidatesForTest(index, catalog, NewRegistry(), duplicate); err == nil || !strings.Contains(err.Error(), "duplicate requirement result") {
		t.Fatalf("duplicate evidence IDs should be rejected, got %v", err)
	}
}
