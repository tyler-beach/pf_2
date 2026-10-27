package evidence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func TestJSONFactProducerEmitsCanonicalConfigValues(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "service.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"timeout_seconds":30,"retry_limit":3,"extra":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	producer := JSONFactProducer{RequiredKeys: []string{"timeout_seconds", "retry_limit"}, NumericKeys: []string{"timeout_seconds"}}
	bundle, err := producer.Collect(root, "config/service.json", "run-json")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ScanCompleteness != "COMPLETE" || len(bundle.Records) != 2 {
		t.Fatalf("unexpected bundle: %+v", bundle)
	}
	for _, rec := range bundle.Records {
		if rec.Kind != KindConfigValue || rec.Status != StatusObserved {
			t.Fatalf("unexpected record: %+v", rec)
		}
		if rec.Producer.AdapterVersion != JSONFactAdapterVersion {
			t.Fatalf("adapter version = %q", rec.Producer.AdapterVersion)
		}
	}
	ids := StableEvidenceIDs(bundle)
	again, err := producer.Collect(root, "config/service.json", "run-json")
	if err != nil {
		t.Fatal(err)
	}
	againIDs := StableEvidenceIDs(again)
	if len(ids) != len(againIDs) || ids[0] != againIDs[0] {
		t.Fatalf("json-fact IDs not stable: %v vs %v", ids, againIDs)
	}
}

func TestTwoProducersShareEnvelopeContract(t *testing.T) {
	opengrep := FromScanResult(analyzer.ScanResult{
		SchemaVersion: "1", Status: analyzer.ScanStatusComplete,
		Analyzers:       []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.30.0"}},
		InputSHA256:     "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RawOutputSHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		ScannedPaths:    []string{"a.go"},
		Findings: []analyzer.Finding{{
			RuleID: "rules.pathfinder.go-shell-command", Path: "a.go", StartLine: 1,
			Fingerprint: "fp", Analyzer: "opengrep",
		}},
	}, "run-og")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "svc.json"), []byte(`{"timeout_seconds":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	jsonFacts, err := (JSONFactProducer{RequiredKeys: []string{"timeout_seconds"}}).Collect(root, "svc.json", "run-jf")
	if err != nil {
		t.Fatal(err)
	}
	if opengrep.SchemaVersion != SchemaVersion || jsonFacts.SchemaVersion != SchemaVersion {
		t.Fatalf("schema mismatch: %s vs %s", opengrep.SchemaVersion, jsonFacts.SchemaVersion)
	}
	if len(opengrep.Records) == 0 || len(jsonFacts.Records) == 0 {
		t.Fatal("both producers must emit records")
	}
	if opengrep.Records[0].Producer.Analyzer == jsonFacts.Records[0].Producer.Analyzer {
		t.Fatal("expected distinct producers")
	}
}
