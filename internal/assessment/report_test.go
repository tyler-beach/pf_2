package assessment

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func TestSARIFReportIsDeterministicAndPreservesFingerprint(t *testing.T) {
	result := Result{
		Requirements: []RequirementResult{{
			ADR: "ADR-003", ADRVersion: 1, RequirementID: "REQ-003-01",
			Title: "No dynamic shell", Status: "FAIL", Severity: "error",
			Findings: []Finding{{
				RuleID: "pathfinder.go-shell-command", Path: "internal/run.go", Line: 14,
				Column: 5, Message: "Avoid shell invocation", Analyzer: "opengrep",
				AnalyzerVersion: "1.30.0", Fingerprint: "stable-fingerprint",
			}},
		}},
	}
	first, err := SARIFReport(result)
	if err != nil { t.Fatal(err) }
	second, err := SARIFReport(result)
	if err != nil { t.Fatal(err) }
	if string(first) != string(second) { t.Fatal("SARIF output is not deterministic") }
	if !strings.Contains(string(first), "stable-fingerprint") { t.Fatal("SARIF omitted finding fingerprint") }
	var decoded map[string]any
	if err := json.Unmarshal(first, &decoded); err != nil { t.Fatalf("invalid SARIF JSON: %v", err) }
	runs := decoded["runs"].([]any)
	run := runs[0].(map[string]any)
	results := run["results"].([]any)
	physical := results[0].(map[string]any)["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	region := physical["region"].(map[string]any)
	if region["startColumn"] != float64(5) { t.Fatalf("SARIF column lost: %+v", region) }
}

func TestMarkdownReportIncludesAnalyzerProvenance(t *testing.T) {
	result := Result{
		Verdict: "FAIL", CoverageStatus: "COMPLETE", EvidenceMode: string(EvidenceModeRepository),
		EngineVersion: "0.2.0", AnalyzerResults: []analyzer.ScanResult{{
			Status: "COMPLETE", Analyzers: []analyzer.AnalyzerInfo{{Name:"opengrep", Version:"1.30.0"}},
			InputSHA256: strings.Repeat("i",64), ConfigurationSHA256: strings.Repeat("c",64), RawOutputSHA256: strings.Repeat("r",64),
		}},
	}
	report := MarkdownReport(result)
	for _, want := range []string{"opengrep", "1.30.0", strings.Repeat("i",64), strings.Repeat("c",64), strings.Repeat("r",64)} {
		if !strings.Contains(report, want) { t.Errorf("Markdown report missing %q", want) }
	}
}
