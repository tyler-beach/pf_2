package evidence

import (
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func TestFromScanResultIsDeterministic(t *testing.T) {
	scan := analyzer.ScanResult{
		SchemaVersion: "1", Status: analyzer.ScanStatusComplete,
		Analyzers:           []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.30.0"}},
		ConfigurationPath:   "rules/pathfinder-security.yml",
		ConfigurationSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BinarySHA256:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		RuleSetSHA256:       "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		InputSHA256:         "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RawOutputSHA256:     "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		ScannedPaths:        []string{"cmd/adrctl/main.go"},
		Findings: []analyzer.Finding{{
			Analyzer: "opengrep", AnalyzerVersion: "1.30.0",
			RuleID: "rules.pathfinder.go-shell-command", Message: "shell",
			Path: "cmd/adrctl/main.go", StartLine: 10, StartColumn: 1, Level: "error",
			Fingerprint: "fp-1",
		}},
	}
	one := FromScanResult(scan, "run-a")
	two := FromScanResult(scan, "run-a")
	if one.Records[0].EvidenceID != two.Records[0].EvidenceID {
		t.Fatalf("evidence IDs diverge: %q vs %q", one.Records[0].EvidenceID, two.Records[0].EvidenceID)
	}
	if one.Records[0].Kind != KindScannerFinding || one.Records[0].Status != StatusObserved {
		t.Fatalf("unexpected record: %+v", one.Records[0])
	}
	if one.Records[0].Producer.AdapterVersion != "opengrep-sarif" {
		t.Fatalf("expected opengrep-sarif adapter, got %q", one.Records[0].Producer.AdapterVersion)
	}
}

func TestIncompleteScanNeverObservedOnly(t *testing.T) {
	scan := analyzer.ScanResult{
		SchemaVersion: "1", Status: analyzer.ScanStatusPartial,
		Analyzers:       []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.30.0"}},
		InputSHA256:     "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RawOutputSHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Errors:          []string{"timeout"},
		Findings: []analyzer.Finding{{
			RuleID: "rules.pathfinder.go-shell-command", Message: "shell",
			Path: "cmd/main.go", StartLine: 1, Fingerprint: "fp-2",
		}},
	}
	bundle := FromScanResult(scan, "run-b")
	if bundle.ScanCompleteness != analyzer.ScanStatusPartial {
		t.Fatalf("completeness = %s", bundle.ScanCompleteness)
	}
	for _, rec := range bundle.Records {
		if rec.Kind == KindScannerFinding && rec.Status == StatusObserved {
			t.Fatalf("partial scan must not mark findings as cleanly observed: %+v", rec)
		}
	}
}
