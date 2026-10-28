package obligations

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

// CandidateAssessment ties a reviewed candidate to the existing deterministic
// requirement result. A candidate's interpretation status is separate from the
// code-evidence status, so an unreviewed candidate cannot accidentally pass.
type CandidateAssessment struct {
	CandidateID       string   `json:"candidate_id"`
	ADRID             string   `json:"adr_id"`
	ADRTitle          string   `json:"adr_title"`
	SectionHeading    string   `json:"section_heading"`
	Statement         string   `json:"statement"`
	Modality          string   `json:"modality"`
	Kind              string   `json:"kind"`
	PatternHint        string   `json:"pattern_hint"`
	ReviewHints        []string `json:"review_hints,omitempty"`
	SourcePath         string   `json:"source_path"`
	SourceSHA256       string   `json:"source_sha256"`
	SourceLineStart    int      `json:"source_line_start"`
	SourceLineEnd      int      `json:"source_line_end"`
	Confidence         string   `json:"confidence,omitempty"`
	ConfidenceP        float64  `json:"confidence_p,omitempty"`
	ConfidenceMethod   string   `json:"confidence_method,omitempty"`
	ConfidenceCoverage float64  `json:"confidence_coverage,omitempty"`
	Decision           string   `json:"review_decision"`
	RequirementID      string   `json:"requirement_id,omitempty"`
	RequirementSHA256  string   `json:"requirement_sha256,omitempty"`
	PatternID          string   `json:"pattern_id,omitempty"`
	EnforcementMode    string   `json:"enforcement_mode"`
	Status             string   `json:"status"`
	EvidenceStatus     string   `json:"evidence_status"`
	Evidence           []string `json:"evidence"`
	Reason             string   `json:"reason"`
	EvidenceWeight     float64  `json:"evidence_weight,omitempty"`
}

// AssessmentReport is a deterministic join of source-cited candidates, the
// append-only review ledger, and a repository-mode assessment result.
type AssessmentReport struct {
	SchemaVersion                   string                `json:"schema_version"`
	EvidenceMode                    string                `json:"evidence_mode"`
	RepositoryVerdict               string                `json:"repository_verdict"`
	ObligationVerdict               string                `json:"obligation_verdict"`
	AssessmentSourceSnapshotSHA256  string                `json:"assessment_source_snapshot_sha256,omitempty"`
	CurrentSourceSnapshotSHA256     string                `json:"current_source_snapshot_sha256,omitempty"`
	CurrentRepositorySnapshotSHA256 string                `json:"current_repository_snapshot_sha256,omitempty"`
	EvidenceFresh                   bool                  `json:"evidence_fresh"`
	CandidateIndexArtifactSHA256    string                `json:"candidate_index_artifact_sha256,omitempty"`
	ReviewRegistryArtifactSHA256    string                `json:"review_registry_artifact_sha256,omitempty"`
	AssessmentArtifactSHA256        string                `json:"assessment_artifact_sha256,omitempty"`
	CandidateCount                  int                   `json:"candidate_count"`
	Counts                          map[string]int        `json:"counts"`
	BindingCoverage                 BindingCoverage       `json:"binding_coverage"`
	Candidates                      []CandidateAssessment `json:"candidates"`
	Warnings                        []string              `json:"warnings,omitempty"`
}

// AssessCandidates joins each current candidate to its latest human decision
// and, only when an approved typed binding exists, to the matching requirement
// result. Missing review or evidence is represented as INDETERMINATE.
func AssessCandidates(index Index, catalog assessment.Catalog, registry Registry, evidence assessment.Result, currentSourceSnapshotSHA256, currentRepositorySnapshotSHA256 string) (AssessmentReport, error) {
	out := AssessmentReport{
		SchemaVersion:     "1",
		EvidenceMode:      evidence.EvidenceMode,
		RepositoryVerdict: evidence.Verdict,
		CandidateCount:    len(index.Candidates),
		Counts:            map[string]int{"PASS": 0, "FAIL": 0, "INDETERMINATE": 0, "NOT_APPLICABLE": 0},
		Candidates:        []CandidateAssessment{},
		Warnings:          []string{},
	}
	if evidence.EvidenceMode != string(assessment.EvidenceModeRepository) {
		return out, fmt.Errorf("obligation assessment requires repository-mode evidence, got %q", evidence.EvidenceMode)
	}
	if evidence.LLMCalls != 0 {
		return out, fmt.Errorf("obligation assessment rejects evidence with %d runtime LLM call(s); deterministic evidence is required", evidence.LLMCalls)
	}
	if !validSHA256(currentSourceSnapshotSHA256) || !validSHA256(currentRepositorySnapshotSHA256) {
		return out, fmt.Errorf("current source and repository snapshot digests must be valid SHA-256 values")
	}
	out.AssessmentSourceSnapshotSHA256 = evidence.SourceSnapshotSHA256
	out.CurrentSourceSnapshotSHA256 = currentSourceSnapshotSHA256
	out.CurrentRepositorySnapshotSHA256 = currentRepositorySnapshotSHA256
	sourceFresh := validSHA256(evidence.SourceSnapshotSHA256) && strings.EqualFold(evidence.SourceSnapshotSHA256, currentSourceSnapshotSHA256)
	repositoryFresh := true
	for _, scan := range evidence.AnalyzerResults {
		if !validSHA256(scan.InputSHA256) || !strings.EqualFold(scan.InputSHA256, currentRepositorySnapshotSHA256) {
			repositoryFresh = false
			break
		}
	}
	out.EvidenceFresh = sourceFresh && repositoryFresh
	coverage, err := ValidateRegistry(index, catalog, registry)
	if err != nil {
		return out, fmt.Errorf("validate obligation review registry: %w", err)
	}
	out.BindingCoverage = coverage
	if coverage.StaleCount > 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf("%d stale candidate review or verifier binding snapshot(s) require re-review", coverage.StaleCount))
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
			CandidateID: candidate.ID, ADRID: candidate.ADRID, ADRTitle: candidate.ADRTitle,
			SectionHeading: candidate.SectionHeading, Statement: candidate.Statement,
			Modality: candidate.Modality, Kind: candidate.Kind, PatternHint: candidate.PatternHint,
			ReviewHints: append([]string(nil), candidate.ReviewHints...),
			SourcePath:  candidate.SourcePath, SourceSHA256: candidate.SourceSHA256,
			SourceLineStart: candidate.SourceLineStart, SourceLineEnd: candidate.SourceLineEnd,
			Confidence: candidate.Confidence, ConfidenceP: candidate.ConfidenceP,
			ConfidenceMethod: candidate.ConfidenceMethod, ConfidenceCoverage: candidate.ConfidenceCoverage,
			Decision: DecisionPending, EnforcementMode: EnforcementReviewRequired,
			Status: "INDETERMINATE", EvidenceStatus: "MISSING", Evidence: []string{},
			Reason: "candidate has not been reviewed; no compliance conclusion is available",
		}
		record, found := latest[candidate.ID]
		if found {
			item.Decision = record.Decision
			item.RequirementID = record.RequirementID
			item.RequirementSHA256 = record.RequirementSHA256
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
			if record.RequirementID != "" && record.PatternID != "manual_review" && !bindingSnapshotMatches(record, catalog) {
				item.Status = "INDETERMINATE"
				item.EvidenceStatus = "STALE"
				item.Reason = "approved verifier binding snapshot is stale or missing; re-review the requirement and verifier configuration"
				out.Candidates = append(out.Candidates, item)
				out.Counts[item.Status]++
				continue
			}
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
			item.EvidenceWeight = evidenceWeightForFindings(requirementResult.Findings)
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
	if !out.EvidenceFresh {
		out.RepositoryVerdict = "INDETERMINATE"
		out.Warnings = append(out.Warnings, "assessment source or repository snapshot does not match current inputs; rerun assess-repo before relying on compliance results")
		for i := range out.Candidates {
			item := &out.Candidates[i]
			if item.Decision == DecisionApproved && item.RequirementID != "" && item.PatternID != "manual_review" {
				item.Status = "INDETERMINATE"
				item.EvidenceStatus = "STALE"
				item.Reason = "assessment evidence is stale for the current source/repository snapshot; rerun assess-repo"
			}
		}
		out.Counts = map[string]int{"PASS": 0, "FAIL": 0, "INDETERMINATE": 0, "NOT_APPLICABLE": 0}
		for _, item := range out.Candidates {
			out.Counts[item.Status]++
		}
	}
	out.ObligationVerdict = deriveObligationVerdict(out.Counts)
	sort.Strings(out.Warnings)
	return out, nil
}

func deriveObligationVerdict(counts map[string]int) string {
	if counts["FAIL"] > 0 {
		return "FAIL"
	}
	if counts["INDETERMINATE"] > 0 {
		return "INDETERMINATE"
	}
	if counts["PASS"] > 0 {
		return "PASS"
	}
	return "NOT_APPLICABLE"
}

// MarkdownAssessmentReport renders a compact audit-friendly per-obligation view.
func MarkdownAssessmentReport(report AssessmentReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Specification obligation assessment\n\n")
	fmt.Fprintf(&b, "- Obligation verdict: **%s**\n", report.ObligationVerdict)
	fmt.Fprintf(&b, "- Repository assessment verdict: **%s**\n", report.RepositoryVerdict)
	fmt.Fprintf(&b, "- Evidence freshness: **%t**\n", report.EvidenceFresh)
	if report.AssessmentSourceSnapshotSHA256 != "" {
		fmt.Fprintf(&b, "- Assessment source snapshot SHA-256: `%s`\n", report.AssessmentSourceSnapshotSHA256)
	}
	if report.CurrentSourceSnapshotSHA256 != "" {
		fmt.Fprintf(&b, "- Current source snapshot SHA-256: `%s`\n", report.CurrentSourceSnapshotSHA256)
	}
	if report.CurrentRepositorySnapshotSHA256 != "" {
		fmt.Fprintf(&b, "- Current repository snapshot SHA-256: `%s`\n", report.CurrentRepositorySnapshotSHA256)
	}
	fmt.Fprintf(&b, "- Candidates: **%d**\n", report.CandidateCount)
	if report.CandidateIndexArtifactSHA256 != "" {
		fmt.Fprintf(&b, "- Candidate index SHA-256: `%s`\n", report.CandidateIndexArtifactSHA256)
	}
	if report.ReviewRegistryArtifactSHA256 != "" {
		fmt.Fprintf(&b, "- Review registry SHA-256: `%s`\n", report.ReviewRegistryArtifactSHA256)
	}
	if report.AssessmentArtifactSHA256 != "" {
		fmt.Fprintf(&b, "- Assessment artifact SHA-256: `%s`\n", report.AssessmentArtifactSHA256)
	}
	fmt.Fprintf(&b, "- PASS: %d; FAIL: %d; INDETERMINATE: %d; NOT_APPLICABLE: %d\n\n",
		report.Counts["PASS"], report.Counts["FAIL"], report.Counts["INDETERMINATE"], report.Counts["NOT_APPLICABLE"])
	if len(report.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, warning := range report.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
		b.WriteString("\n")
	}
	b.WriteString("| Candidate | Modality | Review hints | Status | Review | Requirement | Binding snapshot SHA-256 | Source | Reason |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, candidate := range report.Candidates {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | `%s` | %s:%d-%d | %s |\n",
			mdCell(candidate.CandidateID), mdCell(candidate.Modality), mdCell(strings.Join(candidate.ReviewHints, ", ")),
			mdCell(candidate.Status), mdCell(candidate.Decision), mdCell(candidate.RequirementID), mdCell(candidate.RequirementSHA256),
			mdCell(candidate.SourcePath), candidate.SourceLineStart, candidate.SourceLineEnd, mdCell(candidate.Reason))
	}
	return b.String()
}

func evidenceWeightForFindings(findings []assessment.Finding) float64 {
	table := calibration.DefaultAnalyzerTable()
	if len(findings) == 0 {
		return table.DefaultPrecision
	}
	sum := 0.0
	for _, f := range findings {
		sum += calibration.EvidenceWeight(f.RuleID, table)
	}
	return sum / float64(len(findings))
}

func mdCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return value
}
