package assessment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func TestApplicabilityFiltersByExtensionAndKind(t *testing.T) {
	catalog, err := LoadCatalog(filepath.Join("..", "..", "adr", "requirements.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	md, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pass-issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	files := ParseIssueMarkdown(string(md))
	opts := AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindCodeSnippet,
	}
	result := AssessWithOptions(catalog, files, opts)
	if result.Verdict != "PASS" {
		t.Fatalf("pass-issue oracle: want PASS, got %s counts=%v", result.Verdict, result.Counts)
	}
	if result.ApplicableRequirementCount != 4 {
		t.Fatalf("pass-issue applicable count=%d want 4", result.ApplicableRequirementCount)
	}
	applicabilityNA := 0
	for _, rr := range result.Requirements {
		if rr.Status == "NOT_APPLICABLE" && strings.Contains(rr.Reason, ApplicabilityReasonPrefix) {
			applicabilityNA++
		}
	}
	if applicabilityNA < 20 {
		t.Fatalf("expected many applicability N/A for non-.go/.json rules, got %d", applicabilityNA)
	}
}

func TestApplicabilityDeviationIssueFails(t *testing.T) {
	catalog, err := LoadCatalog(filepath.Join("..", "..", "adr", "requirements.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	md, err := os.ReadFile(filepath.Join("..", "..", "testdata", "deviation-issue.md"))
	if err != nil {
		t.Fatal(err)
	}
	result := AssessWithOptions(catalog, ParseIssueMarkdown(string(md)), AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindCodeSnippet,
	})
	if result.Verdict != "FAIL" {
		t.Fatalf("deviation-issue oracle: want FAIL, got %s", result.Verdict)
	}
	if result.Counts["FAIL"] < 2 {
		t.Fatalf("expected ≥2 FAIL, counts=%v", result.Counts)
	}
}

func TestApplicabilityScopeShrinksCorpusSlice(t *testing.T) {
	catalog, err := LoadCatalog(filepath.Join("..", "..", "testdata", "corpus", "requirements.json"))
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	files := []SourceFile{{Path: "app/main.go", Language: "go", Content: "package main\n"}}
	full := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindCodeSnippet,
		AssessAll:      true,
		IssueSurface:   true,
	})
	// Go-only artifact + OpenGrep evidence: Checkov/IaC and non-.go rules N/A by applicability.
	filtered := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindCodeSnippet,
		IssueSurface:   true,
		AnalyzerResults: []analyzer.ScanResult{{
			Status: analyzer.ScanStatusComplete,
			Analyzers: []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.33.0"}},
			InputSHA256: strings.Repeat("a", 64), ConfigurationSHA256: strings.Repeat("b", 64),
			BinarySHA256: strings.Repeat("c", 64), RuleSetSHA256: strings.Repeat("d", 64),
			RawOutputSHA256: strings.Repeat("e", 64),
			ScannedPaths:    []string{"app/main.go"},
		}},
	})
	// Narrow assess --scope that does not intersect corpus "." + go rules' file path
	// still keeps "." rules, but a non-overlapping prefix drops path-scoped rows.
	scopedAway := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindCodeSnippet,
		ScopePrefixes:  []string{"infra/terraform/"},
		IssueSurface:   true,
	})
	if full.RequirementCount != 200 {
		t.Fatalf("full corpus requirement_count=%d", full.RequirementCount)
	}
	applicabilityNA := 0
	evaluatedBeyondApplicability := 0
	for _, rr := range filtered.Requirements {
		if rr.Status == "NOT_APPLICABLE" && strings.Contains(rr.Reason, ApplicabilityReasonPrefix) {
			applicabilityNA++
		} else {
			evaluatedBeyondApplicability++
		}
	}
	if applicabilityNA < 40 {
		t.Fatalf("go-only corpus assess should N/A checkov/non-go rows by applicability, got %d", applicabilityNA)
	}
	if evaluatedBeyondApplicability >= full.RequirementCount {
		t.Fatalf("filtered evaluated=%d should be ≪ full %d", evaluatedBeyondApplicability, full.RequirementCount)
	}
	if evaluatedBeyondApplicability > 160 {
		t.Fatalf("filtered evaluated count %d still too close to full catalog", evaluatedBeyondApplicability)
	}
	scopedNA := 0
	for _, rr := range scopedAway.Requirements {
		if rr.Status == "NOT_APPLICABLE" && strings.Contains(rr.Reason, "assess --scope") {
			scopedNA++
		}
	}
	// Corpus bindings mostly use source_path_prefixes=["."], which intersect any scope.
	// Explicit non-root path rules + json-style rows should still demonstrate scope filtering
	// when present; assert the mechanism fires on a synthetic requirement below.
	_ = scopedNA
}

func TestAssessScopeDropsNonIntersectingPrefixes(t *testing.T) {
	catalog := Catalog{SchemaVersion: "1", ADRs: []ADR{{
		ID: "ADR-S", Title: "S", Version: 1, Status: "accepted",
		Requirements: []Requirement{{
			ID: "REQ-S-01", Title: "domain only", Severity: "error",
			Verifier: Verifier{
				Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"rules.s"},
				SourcePathPrefixes: []string{"internal/domain/"}, SourceExtensions: []string{".go"},
			},
		}},
	}}}
	files := []SourceFile{{Path: "internal/domain/x.go", Language: "go", Content: "package domain\n"}}
	miss := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode: EvidenceModeSubmission, SubmissionKind: SubmissionKindCodeSnippet,
		ScopePrefixes: []string{"cmd/"},
	})
	if miss.Requirements[0].Status != "NOT_APPLICABLE" || !strings.Contains(miss.Requirements[0].Reason, "assess --scope") {
		t.Fatalf("scope cmd/ should N/A domain rule: %+v", miss.Requirements[0])
	}
	hit := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode: EvidenceModeSubmission, SubmissionKind: SubmissionKindCodeSnippet,
		ScopePrefixes: []string{"internal/"},
	})
	if strings.Contains(hit.Requirements[0].Reason, ApplicabilityReasonPrefix) && strings.Contains(hit.Requirements[0].Reason, "assess --scope") {
		t.Fatalf("scope internal/ should intersect domain rule: %+v", hit.Requirements[0])
	}
}

func TestTechSpecMarksCodeRulesNotApplicableByKind(t *testing.T) {
	catalog := Catalog{SchemaVersion: "1", ADRs: []ADR{{
		ID: "ADR-X", Title: "X", Version: 1, Status: "accepted",
		Requirements: []Requirement{
			{ID: "REQ-X-01", Title: "shell", Severity: "error", Verifier: Verifier{
				Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"rules.x"},
				SourcePathPrefixes: []string{"."}, SourceExtensions: []string{".go"},
			}},
			{ID: "REQ-X-02", Title: "manual", Severity: "warning", Verifier: Verifier{Type: "manual-review"}},
		},
	}}}
	result := AssessWithOptions(catalog, nil, AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindTechSpec,
		IssueSurface:   true,
	})
	if result.Requirements[0].Status != "NOT_APPLICABLE" || !strings.Contains(result.Requirements[0].Reason, "submission_kind") {
		t.Fatalf("code rule should be N/A by kind: %+v", result.Requirements[0])
	}
	// Manual still cleared by IssueSurface (not kind filter — manuals allow tech_spec).
	if result.Requirements[1].Status != "NOT_APPLICABLE" {
		t.Fatalf("manual on issue surface should be N/A: %+v", result.Requirements[1])
	}
	if strings.Contains(result.Requirements[1].Reason, ApplicabilityReasonPrefix) {
		t.Fatalf("manual N/A should keep IssueSurface semantics, got %q", result.Requirements[1].Reason)
	}
}

func TestAssessAllBypassesApplicabilityFilter(t *testing.T) {
	catalog := Catalog{SchemaVersion: "1", ADRs: []ADR{{
		ID: "ADR-Y", Title: "Y", Version: 1, Status: "accepted",
		Requirements: []Requirement{{
			ID: "REQ-Y-01", Title: "py only", Severity: "error",
			Verifier: Verifier{
				Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"rules.y"},
				SourcePathPrefixes: []string{"."}, SourceExtensions: []string{".py"},
			},
		}},
	}}}
	files := []SourceFile{{Path: "main.go", Language: "go", Content: "package main"}}
	filtered := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode: EvidenceModeSubmission, SubmissionKind: SubmissionKindCodeSnippet,
	})
	if !strings.Contains(filtered.Requirements[0].Reason, ApplicabilityReasonPrefix) {
		t.Fatalf("without assess-all expected applicability N/A, got %q", filtered.Requirements[0].Reason)
	}
	all := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode: EvidenceModeSubmission, SubmissionKind: SubmissionKindCodeSnippet, AssessAll: true,
	})
	if strings.Contains(all.Requirements[0].Reason, ApplicabilityReasonPrefix) {
		t.Fatalf("assess-all must not use applicability filter: %q", all.Requirements[0].Reason)
	}
	if all.Requirements[0].Status != "NOT_APPLICABLE" {
		t.Fatalf("assess-all still uses submission analyzer N/A: %+v", all.Requirements[0])
	}
}

func TestAnalyzerCapabilityFiltersCheckovWhenOnlyOpenGrepPresent(t *testing.T) {
	catalog := Catalog{SchemaVersion: "1", ADRs: []ADR{{
		ID: "ADR-Z", Title: "Z", Version: 1, Status: "accepted",
		Requirements: []Requirement{{
			ID: "REQ-Z-01", Title: "iac", Severity: "error",
			Verifier: Verifier{
				Type: "analyzer-rule", Analyzer: "checkov", RuleIDs: []string{"CKV_AWS_1"},
				SourcePathPrefixes: []string{"."}, SourceExtensions: []string{".tf"},
			},
		}},
	}}}
	files := []SourceFile{{Path: "main.tf", Language: "hcl", Content: "resource \"aws_s3_bucket\" \"b\" {}\n"}}
	result := AssessWithOptions(catalog, files, AssessmentOptions{
		EvidenceMode:   EvidenceModeRepository,
		SubmissionKind: SubmissionKindCodeSnippet,
		AnalyzerResults: []analyzer.ScanResult{{
			Status: analyzer.ScanStatusComplete,
			Analyzers: []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.33.0"}},
			InputSHA256: strings.Repeat("a", 64), ConfigurationSHA256: strings.Repeat("b", 64),
			BinarySHA256: strings.Repeat("c", 64), RuleSetSHA256: strings.Repeat("d", 64),
			RawOutputSHA256: strings.Repeat("e", 64),
			ScannedPaths:    []string{"main.tf"},
		}},
	})
	if result.Requirements[0].Status != "NOT_APPLICABLE" || !strings.Contains(result.Requirements[0].Reason, "analyzer") {
		t.Fatalf("checkov rule should be N/A by analyzer capability: %+v", result.Requirements[0])
	}
}
