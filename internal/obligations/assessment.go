package obligations

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

// CandidateAssessment ties a reviewed candidate to the existing deterministic
// requirement result. A candidate's interpretation status is separate from the
// code-evidence status, so an unreviewed candidate cannot accidentally pass.
type CandidateAssessment struct {
	CandidateID     string   `json:"candidate_id"`
	ADRID           string   `json:"adr_id"`
	Statement       string   `json:"statement"`
	SourcePath      string   `json:"source_path"`
	SourceSHA256    string   `json:"source_sha256"`
	SourceLineStart int      `json:"source_line_start"`
	SourceLineEnd   int      `json:"source_line_end"`
	Decision        string   `json:"review_decision"`
	RequirementID   string   `json:"requirement_id,omitempty"`
	PatternID       string   `json:"pattern_id,omitempty"`
	EnforcementMode string   `json:"enforcement_mode"`
	Status          string   `json:"status"`
	EvidenceStatus  string   `json:"evidence_status"`
	Evidence        []string `json:"evidence"`
	Reason          string   `json:"reason"`
}

// AssessmentReport is a deterministic join of source-cited candidates, the
// append-only review ledger, and a repository-mode assessment result.
type AssessmentReport struct {
	SchemaVersion       string                `json:"schema_version"`
	EvidenceMode        string                `json:"evidence_mode"`
	RepositoryVerdict   string                `json:"repository_verdict"`
	CandidateIndexArtifactSHA256 string `json:"candidate_index_artifact_sha256,omitempty"`
	ReviewRegistryArtifactSHA256 string `json:"review_registry_artifact_sha256,omitempty"`
	AssessmentArtifactSHA256 string `json:"assessment_artifact_sha256,omitempty"`
	CandidateCount      int                   `json:"candidate_count"`
	Counts              map[string]int        `json:"counts"`
	BindingCoverage     BindingCoverage       `json:"binding_coverage"`
	Candidates          []CandidateAssessment `json:"candidates"`
	Warnings            []string              `json:"warnings,omitempty"`
}

// AssessCandidates joins each current candidate to its latest human decision
// and, only when an approved typed binding exists, to the matching requirement
// result. Missing review or evidence is represented as INDETERMINATE.
func AssessCandidates(index Index, catalog assessment.Catalog, registry Registry, evidence assessment.Result) (AssessmentReport, error) {
	out := AssessmentReport{
		SchemaVersion: "1",
		EvidenceMode: evidence.EvidenceMode,
		RepositoryVerdict: evidence.Verdict,
		CandidateCount: len(index.Candidates),
		Counts: map[string]int{"PASS": 0, "FAIL": 0, "INDETERMINATE": 0, "NOT_APPLICABLE": 0},
		Candidates: []CandidateAssessment{},
		Warnings: []string{},
	}
	if evidence.EvidenceMode != string(assessment.EvidenceModeRepository) {
		return out, fmt.Errorf("obligation assessment requires repository-mode evidence, got %q", evidence.EvidenceMode)
	}
	coverage, err := ValidateRegistry(index, catalog, registry)
	if err != nil {
		return out, fmt.Errorf("validate obligation review registry: %w", err)
	}
	out.BindingCoverage = coverage
	if coverage.StaleCount > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d stale candidate review(s) require re-review", coverage.StaleCount))
	}
	if coverage.OrphanedCount > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d orphaned review history record(s) require explicit supersession", coverage.OrphanedCount))
	}

	requirements := make(map[string]assessment.RequirementResult, len(evidence.Requirements))
	for _, result := range evidence.Requirements {
		if _, exists := requirements[result.RequirementID]; exists {
			return out, fmt.Errorf("assessment contains duplicate requirement result %q", result.RequirementID)
		}
		requirements[result.RequirementID] = result
	}
	latest := latestRecords(registry)
	for _, candidate := range index.Candidates {
		item := CandidateAssessment{
			CandidateID: candidate.ID, ADRID: candidate.ADRID, Statement: candidate.Statement,
			SourcePath: candidate.SourcePath, SourceSHA256: candidate.SourceSHA256,
			SourceLineStart: candidate.SourceLineStart, SourceLineEnd: candidate.SourceLineEnd,
			Decision: DecisionPending, EnforcementMode: EnforcementReviewRequired,
			Status: "INDETERMINATE", EvidenceStatus: "MISSING", Evidence: []string{},
			Reason: "candidate has not been reviewed; no compliance conclusion is available",
		}
		record, found := latest[candidate.ID]
		if found {
			item.Decision = record.Decision
			item.RequirementID = record.RequirementID
			item.PatternID = record.PatternID
			item.EnforcementMode = record.EnforcementMode
		}
		if !found {
			out.Candidates = append(out.Candidates, item)
			out.Counts[item.Status]++
			continue
		}
		if record.Decision == DecisionSuperseded || !snapshotsMatchCurrent(record.Candidate, candidate) {
			item.Decision = DecisionPending
			item.Status = "INDETERMINATE"
			item.Reason = "review snapshot is stale; re-review the current source-cited candidate"
			out.Candidates = append(out.Candidates, item)
			out.Counts[item.Status]++
			continue
		}
		switch record.Decision {
		case DecisionPending:
			item.Reason = "review is pending; candidate is not an approved obligation"
		case DecisionRejected:
			item.Status = "NOT_APPLICABLE"
			item.Reason = "candidate interpretation was explicitly rejected during review"
		case DecisionApproved:
			if err := validateApprovedBinding(record, candidate, catalog); err != nil {
				return out, fmt.Errorf("approved candidate %s has invalid binding: %w", candidate.ID, err)
			}
			if record.RequirementID == "" || record.PatternID == "manual_review" {
				item.Reason = "approved interpretation has no executable verifier binding; manual review remains required"
				break
			}
			requirementResult, ok := requirements[record.RequirementID]
			if !ok {
				item.EvidenceStatus = "MISSING"
				item.Reason = "bound requirement has no result in the supplied repository assessment"
				break
			}
			item.EvidenceStatus = requirementResult.Status
			item.Evidence = append([]string(nil), requirementResult.Evidence...)
			if (requirementResult.Status == "PASS" || requirementResult.Status == "FAIL") &&
				len(requirementResult.Evidence) == 0 && len(requirementResult.Findings) == 0 {
				item.Status = "INDETERMINATE"
				item.EvidenceStatus = "MISSING"
				item.Reason = "requirement result has no evidence supporting its compliance status"
				break
			}
			if requirementResult.Reason != "" {
				item.Reason = requirementResult.Reason
			} else {
				item.Reason = "status inherited from the matching deterministic requirement assessment"
			}
			switch requirementResult.Status {
			case "PASS", "FAIL", "NOT_APPLICABLE":
				item.Status = requirementResult.Status
			default:
				item.Status = "INDETERMINATE"
				if item.Reason == "" {
					item.Reason = "the bound requirement lacks complete evidence"
				}
			}
			if record.EnforcementMode == EnforcementReviewRequired {
				item.Status = "INDETERMINATE"
				item.Reason = "binding is approved but enforcement_mode is review_required; " + item.Reason
			}
		case DecisionSuperseded:
			item.Reason = "review history was superseded; current candidate requires a new review"
		}
		out.Candidates = append(out.Candidates, item)
		out.Counts[item.Status]++
	}
	sort.Strings(out.Warnings)
	return out, nil
}

// MarkdownAssessmentReport renders a compact audit-friendly per-obligation view.
func MarkdownAssessmentReport(report AssessmentReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Specification obligation assessment\n\n")
	fmt.Fprintf(&b, "- Repository assessment verdict: **%s**\n", report.RepositoryVerdict)
	fmt.Fprintf(&b, "- Candidates: **%d**\n", report.CandidateCount)
	if report.CandidateIndexArtifactSHA256 != "" { fmt.Fprintf(&b, "- Candidate index SHA-256: `%s`\n", report.CandidateIndexArtifactSHA256) }
	if report.ReviewRegistryArtifactSHA256 != "" { fmt.Fprintf(&b, "- Review registry SHA-256: `%s`\n", report.ReviewRegistryArtifactSHA256) }
	if report.AssessmentArtifactSHA256 != "" { fmt.Fprintf(&b, "- Assessment artifact SHA-256: `%s`\n", report.AssessmentArtifactSHA256) }
	fmt.Fprintf(&b, "- PASS: %d; FAIL: %d; INDETERMINATE: %d; NOT_APPLICABLE: %d\n\n",
		report.Counts["PASS"], report.Counts["FAIL"], report.Counts["INDETERMINATE"], report.Counts["NOT_APPLICABLE"])
	if len(report.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, warning := range report.Warnings { fmt.Fprintf(&b, "- %s\n", warning) }
		b.WriteString("\n")
	}
	b.WriteString("| Candidate | Status | Review | Requirement | Source | Reason |\n|---|---|---|---|---|---|\n")
	for _, candidate := range report.Candidates {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s:%d-%d | %s |\n",
			mdCell(candidate.CandidateID), mdCell(candidate.Status), mdCell(candidate.Decision),
			mdCell(candidate.RequirementID), mdCell(candidate.SourcePath), candidate.SourceLineStart,
			candidate.SourceLineEnd, mdCell(candidate.Reason))
	}
	return b.String()
}

func mdCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return value
}
