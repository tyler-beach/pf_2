package obligations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

func reviewFixture() (Index, assessment.Catalog, Candidate) {
	candidate := Candidate{
		ADRID: "ADR-001",
		ADRTitle: "Preserve domain boundaries",
		ADRStatus: "accepted",
		SectionHeading: "Decision",
		Statement: "Domain code must not import internal infrastructure packages.",
		Modality: "must_not",
		Kind: "normative_candidate",
		PatternHint: "architecture.dependency_or_boundary",
		SourcePath: "adrs/ADR-001-boundaries.md",
		SourceSHA256: strings.Repeat("a", 64),
		SourceLineStart: 12,
		SourceLineEnd: 12,
		ApprovalStatus: DecisionPending,
		EnforcementMode: EnforcementReviewRequired,
		Executable: false,
	}
	candidate.ID = candidateID(candidate)
	index := Index{
		SchemaVersion: "1",
		SourceRoot: "adrs",
		SourceIndexSHA256: strings.Repeat("b", 64),
		DocumentCount: 1,
		DocumentsWithCandidates: 1,
		CandidateCount: 1,
		Candidates: []Candidate{candidate},
	}
	min := 1.0
	_ = min
	catalog := assessment.Catalog{
		SchemaVersion: "1",
		ADRs: []assessment.ADR{{
			ID: "ADR-001", Title: "Preserve domain boundaries", Version: 1, Status: "accepted",
			Requirements: []assessment.Requirement{{
				ID: "REQ-001-01", Title: "Domain code must not depend on infrastructure adapters",
				Description: "Domain source files must not import internal infrastructure packages.",
				Severity: "error",
				Verifier: assessment.Verifier{
					Type: "go-import-boundary",
					SourcePathPrefixes: []string{"internal/domain/"},
					ForbiddenImportPrefixes: []string{"internal/infrastructure/"},
				},
			}},
		}},
	}
	return index, catalog, candidate
}

func validReviewProposal(candidate Candidate) ReviewProposal {
	return ReviewProposal{
		CandidateID: candidate.ID,
		Decision: DecisionApproved,
		RequirementID: "REQ-001-01",
		PatternID: candidate.PatternHint,
		EnforcementMode: EnforcementBlocking,
		Reviewer: "security-architect@example.com",
		ReviewedAt: "2026-10-27T12:00:00Z",
		ReviewReference: "https://github.com/tyler-beach/pf_2/issues/20",
		Rationale: "The ADR statement is covered by the existing, tested Go import-boundary requirement.",
	}
}

func TestRecordReviewCreatesSourceBoundBlockingBinding(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	registry, coverage, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil { t.Fatal(err) }
	if len(registry.Records) != 1 { t.Fatalf("records=%d, want 1", len(registry.Records)) }
	record := registry.Records[0]
	if record.Revision != 1 || record.Decision != DecisionApproved || record.Candidate.SourceSHA256 != candidate.SourceSHA256 {
		t.Fatalf("review record lacks stable source binding: %+v", record)
	}
	if coverage.CandidateCount != 1 || coverage.ApprovedCount != 1 || coverage.BlockingBindingCount != 1 ||
		coverage.PendingReviewCount != 0 || coverage.StaleCount != 0 || coverage.OrphanedCount != 0 || coverage.UnboundCount != 0 {
		t.Fatalf("unexpected binding coverage: %+v", coverage)
	}
}

func TestRecordReviewIsAppendOnlyAndRevisionsAreContiguous(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	pending := ReviewProposal{
		CandidateID: candidate.ID, Decision: DecisionPending, EnforcementMode: EnforcementReviewRequired,
		Reviewer: "reviewer@example.com", ReviewedAt: "2026-10-27T12:00:00Z",
		ReviewReference: "https://github.com/tyler-beach/pf_2/issues/20",
		Rationale: "Need confirmation of applicability and exceptions before binding.",
	}
	first, firstCoverage, err := RecordReview(index, catalog, NewRegistry(), pending)
	if err != nil { t.Fatal(err) }
	if firstCoverage.PendingReviewCount != 1 { t.Fatalf("pending coverage=%+v", firstCoverage) }
	proposal := validReviewProposal(candidate)
	second, coverage, err := RecordReview(index, catalog, first, proposal)
	if err != nil { t.Fatal(err) }
	if len(second.Records) != 2 || second.Records[0].Decision != DecisionPending || second.Records[0].Revision != 1 ||
		second.Records[1].Decision != DecisionApproved || second.Records[1].Revision != 2 {
		t.Fatalf("review decisions were not appended as revisions: %+v", second.Records)
	}
	if first.Records[0].Decision != DecisionPending || len(first.Records) != 1 {
		t.Fatalf("RecordReview mutated its input registry: %+v", first)
	}
	if coverage.ApprovedCount != 1 || coverage.BlockingBindingCount != 1 || coverage.PendingReviewCount != 0 {
		t.Fatalf("unexpected revised coverage: %+v", coverage)
	}
}

func TestRecordReviewRejectsInvalidBindingAndAuditMetadata(t *testing.T) {
	cases := []struct {
		name string
		mutate func(*ReviewProposal, *assessment.Catalog)
		want string
	}{
		{"unknown requirement", func(p *ReviewProposal, _ *assessment.Catalog) { p.RequirementID = "REQ-999-01" }, "does not exist"},
		{"incompatible pattern", func(p *ReviewProposal, _ *assessment.Catalog) { p.PatternID = "security.source_pattern" }, "requires an analyzer-rule"},
		{"missing reviewer", func(p *ReviewProposal, _ *assessment.Catalog) { p.Reviewer = "" }, "reviewer is required"},
		{"invalid timestamp", func(p *ReviewProposal, _ *assessment.Catalog) { p.ReviewedAt = "yesterday" }, "RFC3339"},
		{"untrusted reference scheme", func(p *ReviewProposal, _ *assessment.Catalog) { p.ReviewReference = "http://github.com/example/review" }, "HTTPS URL"},
		{"cross-ADR binding", func(p *ReviewProposal, c *assessment.Catalog) {
			c.ADRs = append(c.ADRs, assessment.ADR{
				ID: "ADR-002", Title: "Other decision", Version: 1, Status: "accepted",
				Requirements: []assessment.Requirement{{
					ID: "REQ-002-01", Title: "Other boundary", Severity: "error",
					Verifier: assessment.Verifier{Type:"go-import-boundary", SourcePathPrefixes:[]string{"x/"}, ForbiddenImportPrefixes:[]string{"y/"}},
				}},
			})
			p.RequirementID = "REQ-002-01"
		}, "cannot be bound to requirement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index, catalog, candidate := reviewFixture()
			proposal := validReviewProposal(candidate)
			tc.mutate(&proposal, &catalog)
			if _, _, err := RecordReview(index, catalog, NewRegistry(), proposal); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestManualReviewApprovalCanNeverBecomeBlocking(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	candidate.PatternHint = "manual_review"
	candidate.ID = candidateID(candidate)
	index.Candidates[0] = candidate
	proposal := validReviewProposal(candidate)
	proposal.PatternID = "manual_review"
	proposal.RequirementID = ""
	proposal.EnforcementMode = EnforcementReviewRequired
	registry, coverage, err := RecordReview(index, catalog, NewRegistry(), proposal)
	if err != nil { t.Fatal(err) }
	if coverage.ApprovedCount != 1 || coverage.ReviewRequiredCount != 1 || coverage.BlockingBindingCount != 0 ||
		coverage.UnboundCount != 1 || registry.Records[0].RequirementID != "" {
		t.Fatalf("manual review was incorrectly bound or enforced: coverage=%+v record=%+v", coverage, registry.Records[0])
	}
	proposal.EnforcementMode = EnforcementBlocking
	if _, _, err := RecordReview(index, catalog, NewRegistry(), proposal); err == nil || !strings.Contains(err.Error(), "manual_review") {
		t.Fatalf("manual review must not be blocking, got %v", err)
	}
}

func TestSourceDriftMakesApprovalStaleUntilReReviewed(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	approved, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil { t.Fatal(err) }
	changed := candidate
	changed.SourceSHA256 = strings.Repeat("c", 64)
	index.Candidates[0] = changed
	index.SourceIndexSHA256 = strings.Repeat("d", 64)
	coverage, err := ValidateRegistry(index, catalog, approved)
	if err != nil { t.Fatal(err) }
	if coverage.StaleCount != 1 || coverage.ApprovedCount != 0 || coverage.BlockingBindingCount != 0 ||
		coverage.PendingReviewCount != 1 || coverage.UnboundCount != 1 {
		t.Fatalf("stale source approval was still considered active: %+v", coverage)
	}
	proposal := validReviewProposal(changed)
	revised, coverage, err := RecordReview(index, catalog, approved, proposal)
	if err != nil { t.Fatal(err) }
	if len(revised.Records) != 2 || revised.Records[0].Candidate.SourceSHA256 != candidate.SourceSHA256 ||
		revised.Records[1].Candidate.SourceSHA256 != changed.SourceSHA256 || revised.Records[1].Revision != 2 {
		t.Fatalf("re-review did not preserve old source revision: %+v", revised.Records)
	}
	if coverage.StaleCount != 0 || coverage.ApprovedCount != 1 || coverage.BlockingBindingCount != 1 {
		t.Fatalf("new source revision did not restore active reviewed binding: %+v", coverage)
	}
}

func TestRemovedCandidateMustBeExplicitlySuperseded(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	approved, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil { t.Fatal(err) }
	removed := index
	removed.Candidates = []Candidate{}
	removed.CandidateCount = 0
	removed.SourceIndexSHA256 = strings.Repeat("d", 64)
	if _, err := ValidateActiveRegistry(removed, catalog, approved); err == nil || !strings.Contains(err.Error(), "orphaned candidate") {
		t.Fatalf("removed candidate approval should not silently remain active, got %v", err)
	}
	proposal := ReviewProposal{
		CandidateID: candidate.ID, Decision: DecisionSuperseded,
		EnforcementMode: EnforcementReviewRequired, Reviewer: "security-architect@example.com",
		ReviewedAt: "2026-10-27T12:00:00Z", ReviewReference: "https://github.com/tyler-beach/pf_2/issues/20",
		Rationale: "Source ADR was removed; preserve the audit trail and retire its review binding.",
	}
	superseded, coverage, err := RecordReview(removed, catalog, approved, proposal)
	if err != nil { t.Fatal(err) }
	if len(superseded.Records) != 2 || superseded.Records[1].Revision != 2 ||
		superseded.Records[1].Decision != DecisionSuperseded || coverage.SupersededCount != 1 ||
		coverage.OrphanedCount != 0 || coverage.BlockingBindingCount != 0 {
		t.Fatalf("superseded history was not closed correctly: coverage=%+v registry=%+v", coverage, superseded)
	}
}

func TestRegistryRejectsRevisionGapsAndTamperedSnapshots(t *testing.T) {
	index, catalog, candidate := reviewFixture()
	registry, _, err := RecordReview(index, catalog, NewRegistry(), validReviewProposal(candidate))
	if err != nil { t.Fatal(err) }
	gap := registry
	gap.Records = append([]ReviewRecord{}, registry.Records...)
	gap.Records[0].Revision = 2
	if _, err := ValidateRegistry(index, catalog, gap); err == nil || !strings.Contains(err.Error(), "contiguous revision") {
		t.Fatalf("revision gap should be rejected, got %v", err)
	}
	tampered := registry
	tampered.Records = append([]ReviewRecord{}, registry.Records...)
	tampered.Records[0].Candidate.Statement = "A different statement"
	if _, err := ValidateRegistry(index, catalog, tampered); err == nil || !strings.Contains(err.Error(), "stable source identity") {
		t.Fatalf("tampered source snapshot should be rejected, got %v", err)
	}
}

func TestSaveAndLoadRegistryRoundTripWithRestrictedPermissions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "bindings.json")
	registry := NewRegistry()
	if err := SaveRegistry(path, registry); err != nil { t.Fatal(err) }
	loaded, err := LoadRegistry(path)
	if err != nil { t.Fatal(err) }
	if loaded.SchemaVersion != RegistrySchemaVersion || len(loaded.Records) != 0 {
		t.Fatalf("unexpected round-tripped registry: %+v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0600 { t.Fatalf("registry permissions=%o, want 600", info.Mode().Perm()) }
}
