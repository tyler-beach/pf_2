package assessment

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func TestSARIFReportIsDeterministicAndPreservesFingerprint(t *testing.T) {
	result := Result{
		SchemaVersion:              "1",
		Verdict:                    "FAIL",
		CoverageStatus:             "COMPLETE",
		EvidenceMode:               string(EvidenceModeRepository),
		EngineVersion:              "0.3.0",
		RequirementCount:           1,
		ApplicableRequirementCount: 1,
		Counts:                     map[string]int{"FAIL": 1},
		Requirements: []RequirementResult{{
			ADR: "ADR-003", ADRVersion: 1, RequirementID: "REQ-003-01",
			Title: "No dynamic shell", Status: "FAIL", Severity: "error",
			Findings: []Finding{{
				RuleID: "pathfinder.go-shell-command", Path: "internal/run.go", Line: 14,
				Column: 5, Message: "Avoid shell invocation", Analyzer: "opengrep",
				AnalyzerVersion: "1.33.0", Fingerprint: "stable-fingerprint",
			}},
		}},
	}
	first, err := SARIFReport(result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SARIFReport(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("SARIF output is not deterministic")
	}
	if !strings.Contains(string(first), "stable-fingerprint") {
		t.Fatal("SARIF omitted finding fingerprint")
	}

	var decoded map[string]any
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	runs := decoded["runs"].([]any)
	run := runs[0].(map[string]any)
	properties := run["properties"].(map[string]any)
	metadata := properties["adrctl"].(map[string]any)
	if metadata["verdict"] != "FAIL" || metadata["coverage_status"] != "COMPLETE" {
		t.Fatalf("SARIF assessment metadata missing verdict/coverage: %+v", metadata)
	}
	results := run["results"].([]any)
	resultProperties := results[0].(map[string]any)["properties"].(map[string]any)
	if resultProperties["requirement_id"] != "REQ-003-01" || resultProperties["requirement_status"] != "FAIL" {
		t.Fatalf("SARIF finding did not retain requirement provenance: %+v", resultProperties)
	}
	physical := results[0].(map[string]any)["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	region := physical["region"].(map[string]any)
	if region["startColumn"] != float64(5) {
		t.Fatalf("SARIF column lost: %+v", region)
	}
}

func TestSARIFReportPreservesCoverageAndUnsupportedStates(t *testing.T) {
	result := Result{
		SchemaVersion:              "1",
		Verdict:                    "INDETERMINATE",
		CoverageStatus:             "PARTIAL",
		EvidenceMode:               string(EvidenceModeRepository),
		EngineVersion:              "0.3.0",
		SourceFileCount:            2,
		RequirementCount:           4,
		ApplicableRequirementCount: 1,
		Counts: map[string]int{
			"PASS":             1,
			"UNSUPPORTED":      1,
			"EVIDENCE_MISSING": 1,
			"REVIEW_REQUIRED":  1,
		},
		SourceSnapshotSHA256:  strings.Repeat("s", 64),
		UnassessedSourceFiles: []string{"scripts/evaluate.py (python)"},
		AnalyzerResults: []analyzer.ScanResult{{
			Status:              analyzer.ScanStatusPartial,
			Analyzers:           []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.33.0"}},
			InputSHA256:         strings.Repeat("i", 64),
			ConfigurationSHA256: strings.Repeat("c", 64),
			RawOutputSHA256:     strings.Repeat("r", 64),
			ScannedPaths:        []string{"internal/domain/customer.go"},
			Errors:              []string{"scanner timed out"},
		}},
		Requirements: []RequirementResult{
			{
				ADR: "ADR-001", ADRVersion: 1, RequirementID: "REQ-PASS",
				Title: "Compliant requirement", Status: "PASS", Severity: "error",
				Reason:   "The deterministic verifier passed.",
				Evidence: []string{"internal/domain/customer.go parsed."},
			},
			{
				ADR: "ADR-002", ADRVersion: 1, RequirementID: "REQ-UNSUPPORTED",
				Title: "Unsupported policy", Status: "UNSUPPORTED", Severity: "error",
				Reason: "No approved verifier is registered for type: rego-bundle-v1",
			},
			{
				ADR: "ADR-003", ADRVersion: 1, RequirementID: "REQ-MISSING",
				Title: "Required evidence", Status: "EVIDENCE_MISSING", Severity: "error",
				Reason: "Required configuration file config/service.json was not provided.",
			},
			{
				ADR: "ADR-004", ADRVersion: 1, RequirementID: "REQ-REVIEW",
				Title: "Semantic constraint", Status: "REVIEW_REQUIRED", Severity: "warning",
				Reason: "Evidence requires architecture judgment.",
			},
		},
	}
	data, err := SARIFReport(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}

	run := decoded["runs"].([]any)[0].(map[string]any)
	metadata := run["properties"].(map[string]any)["adrctl"].(map[string]any)
	if metadata["verdict"] != "INDETERMINATE" || metadata["coverage_status"] != "PARTIAL" ||
		metadata["evidence_mode"] != string(EvidenceModeRepository) {
		t.Fatalf("SARIF omitted assessment state: %+v", metadata)
	}
	if metadata["source_snapshot_sha256"] != strings.Repeat("s", 64) {
		t.Fatalf("SARIF omitted source snapshot provenance: %+v", metadata)
	}
	if got := metadata["unassessed_source_files"].([]any); len(got) != 1 || got[0] != "scripts/evaluate.py (python)" {
		t.Fatalf("SARIF omitted unsupported source inventory: %+v", got)
	}

	counts := metadata["status_counts"].(map[string]any)
	for status, want := range map[string]float64{
		"PASS":             1,
		"UNSUPPORTED":      1,
		"EVIDENCE_MISSING": 1,
		"REVIEW_REQUIRED":  1,
	} {
		if counts[status] != want {
			t.Errorf("SARIF count for %s = %v, want %v", status, counts[status], want)
		}
	}

	requirements := metadata["requirement_statuses"].([]any)
	gotStatuses := map[string]string{}
	for _, item := range requirements {
		entry := item.(map[string]any)
		gotStatuses[entry["requirement_id"].(string)] = entry["status"].(string)
	}
	for id, want := range map[string]string{
		"REQ-PASS":        "PASS",
		"REQ-UNSUPPORTED": "UNSUPPORTED",
		"REQ-MISSING":     "EVIDENCE_MISSING",
		"REQ-REVIEW":      "REVIEW_REQUIRED",
	} {
		if gotStatuses[id] != want {
			t.Errorf("SARIF status for %s = %q, want %q", id, gotStatuses[id], want)
		}
	}

	analyzerRuns := metadata["analyzer_runs"].([]any)
	if len(analyzerRuns) != 1 {
		t.Fatalf("got %d analyzer runs, want 1", len(analyzerRuns))
	}
	scan := analyzerRuns[0].(map[string]any)
	if scan["status"] != analyzer.ScanStatusPartial || scan["raw_output_sha256"] != strings.Repeat("r", 64) {
		t.Fatalf("SARIF analyzer provenance missing: %+v", scan)
	}
	analyzers := scan["analyzers"].([]any)
	firstAnalyzer := analyzers[0].(map[string]any)
	if firstAnalyzer["name"] != "opengrep" || firstAnalyzer["version"] != "1.33.0" {
		t.Fatalf("SARIF analyzer identity/version missing: %+v", firstAnalyzer)
	}
}

func TestSARIFReportForEmptyCatalogStillCarriesIndeterminateVerdict(t *testing.T) {
	data, err := SARIFReport(Result{
		SchemaVersion:  "1",
		Verdict:        "INDETERMINATE",
		CoverageStatus: "NONE",
		EvidenceMode:   string(EvidenceModeRepository),
		EngineVersion:  "0.3.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}
	run := decoded["runs"].([]any)[0].(map[string]any)
	metadata := run["properties"].(map[string]any)["adrctl"].(map[string]any)
	if metadata["verdict"] != "INDETERMINATE" || metadata["coverage_status"] != "NONE" {
		t.Fatalf("empty catalog SARIF must not imply PASS: %+v", metadata)
	}
	if len(run["results"].([]any)) != 0 {
		t.Fatalf("empty catalog unexpectedly produced SARIF findings: %+v", run["results"])
	}
}

func TestMarkdownReportIncludesAnalyzerProvenance(t *testing.T) {
	result := Result{
		Verdict: "FAIL", CoverageStatus: "COMPLETE", EvidenceMode: string(EvidenceModeRepository),
		EngineVersion: "0.2.0", AnalyzerResults: []analyzer.ScanResult{{
			Status: "COMPLETE", Analyzers: []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.33.0"}},
			InputSHA256: strings.Repeat("i", 64), ConfigurationSHA256: strings.Repeat("c", 64), RawOutputSHA256: strings.Repeat("r", 64),
		}},
	}
	report := MarkdownReport(result)
	for _, want := range []string{"opengrep", "1.33.0", strings.Repeat("i", 64), strings.Repeat("c", 64), strings.Repeat("r", 64)} {
		if !strings.Contains(report, want) {
			t.Errorf("Markdown report missing %q", want)
		}
	}
}
