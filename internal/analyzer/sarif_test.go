package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSARIFNormalizesFindingsAndMetadata(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{
	  "version":"2.1.0",
	  "runs":[
	    {"tool":{"driver":{"name":"fixture-scanner","version":"1.2.3","rules":[{"id":"RULE-B"},{"id":"RULE-A"}] }},
	     "results":[
	       {"ruleIndex":0,"level":"error","message":{"text":"Second finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/z.go"},"region":{"startLine":8,"startColumn":4}}}]},
	       {"ruleId":"RULE-A","level":"warning","message":{"text":"First finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/a.py"},"region":{"startLine":3,"startColumn":2}}}],"partialFingerprints":{"primaryLocationLineHash":"stable-123"},"properties":{"cwe":"CWE-79"}}
	     ]}
	  ]
	}`)
	got, err := ParseSARIF(data, root)
	if err != nil { t.Fatalf("ParseSARIF returned error: %v", err) }
	if got.Status != ScanStatusComplete { t.Fatalf("Status = %q, want COMPLETE", got.Status) }
	if len(got.Findings) != 2 { t.Fatalf("got %d findings, want 2", len(got.Findings)) }
	if got.Findings[0].Path != "src/a.py" || got.Findings[0].RuleID != "RULE-A" { t.Fatalf("findings not sorted/normalized: %+v", got.Findings) }
	if got.Findings[0].StartLine != 3 || got.Findings[0].StartColumn != 2 { t.Fatalf("location lost: %+v", got.Findings[0]) }
	if got.Findings[0].Fingerprint != "stable-123" { t.Fatalf("source fingerprint not preserved: %+v", got.Findings[0]) }
	if got.Findings[0].Properties["cwe"] != "CWE-79" { t.Fatalf("properties not preserved: %+v", got.Findings[0].Properties) }
	if got.Findings[1].Path != "src/z.go" || got.Findings[1].RuleID != "RULE-B" { t.Fatalf("ruleIndex not resolved: %+v", got.Findings[1]) }
	if got.Findings[1].Fingerprint == "" { t.Fatal("fallback fingerprint is empty") }
	if got.RawOutputSHA256 == "" || len(got.Analyzers) != 1 || got.Analyzers[0].Version != "1.2.3" { t.Fatalf("metadata missing: %+v", got) }
	encoded1, err := WriteJSON(got); if err != nil { t.Fatal(err) }
	encoded2, err := WriteJSON(got); if err != nil { t.Fatal(err) }
	if string(encoded1) != string(encoded2) { t.Fatal("normalized evidence JSON is not deterministic") }
	var decoded ScanResult
	if err := json.Unmarshal(encoded1, &decoded); err != nil { t.Fatalf("normalized evidence is invalid JSON: %v", err) }
}

func TestParseSARIFCombinesMultipleRunsAndSortsAnalyzers(t *testing.T) {
	data := []byte(`{"version":"2.1.0","runs":[
	  {"tool":{"driver":{"name":"z-scanner","version":"2"}},"results":[]},
	  {"tool":{"driver":{"name":"a-scanner","version":"1"}},"results":[]}
	]}`)
	got, err := ParseSARIF(data, t.TempDir())
	if err != nil { t.Fatal(err) }
	if len(got.Analyzers) != 2 || got.Analyzers[0].Name != "a-scanner" || got.Analyzers[1].Name != "z-scanner" {
		t.Fatalf("analyzers not normalized: %+v", got.Analyzers)
	}
}

func TestParseSARIFMarksUnsuccessfulInvocationPartial(t *testing.T) {
	data := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"invocations":[{"executionSuccessful":false,"toolExecutionNotifications":[{"message":{"text":"timed out during one rule"}}]}],"results":[]}]}`)
	got, err := ParseSARIF(data, t.TempDir())
	if err != nil { t.Fatal(err) }
	if got.Status != ScanStatusPartial { t.Fatalf("Status = %q, want PARTIAL", got.Status) }
	if len(got.Errors) != 2 { t.Fatalf("expected invocation failure and notification, got %v", got.Errors) }
}

func TestParseSARIFRejectsInvalidDocuments(t *testing.T) {
	cases := []struct{name, input, want string}{
		{"invalid json", "{", "decode SARIF JSON"},
		{"wrong version", `{"version":"1.0.0","runs":[]}`, "unsupported SARIF version"},
		{"no runs", `{"version":"2.1.0","runs":[]}`, "contains no runs"},
		{"missing driver", `{"version":"2.1.0","runs":[{"tool":{"driver":{}}}]}`, "no tool.driver.name"},
		{"missing rule", `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"x"}},"results":[{"message":{"text":"m"}}]}]}`, "no ruleId"},
		{"missing message", `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"x"}},"results":[{"ruleId":"R","message":{}}]}]}`, "no message"},
		{"bad rule index", `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"x","rules":[{"id":"R"}]}},"results":[{"ruleIndex":5,"message":{"text":"m"}}]}]}`, "out-of-range ruleIndex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSARIF([]byte(tc.input), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tc.want) { t.Fatalf("expected error containing %q, got %v", tc.want, err) }
		})
	}
}

func TestParseSARIFRejectsPathsOutsideRepository(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.py")
	cases := []string{"../secret.py", "%2e%2e/secret.py", outside, "https://example.org/source.py"}
	for _, uri := range cases {
		t.Run(uri, func(t *testing.T) {
			payload := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleId":"R","message":{"text":"finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":` + quote(uri) + `}}}]}]}]}`
			_, err := ParseSARIF([]byte(payload), root)
			if err == nil { t.Fatalf("expected path guard rejection for %q", uri) }
			if !strings.Contains(err.Error(), "artifact") && !strings.Contains(err.Error(), "repository root") && !strings.Contains(err.Error(), "repository evidence") {
				t.Fatalf("unexpected path guard error for %q: %v", uri, err)
			}
		})
	}
}

func TestParseSARIFAllowsAbsolutePathInsideRepository(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "src", "main.go")
	uri := "file://" + filepath.ToSlash(target)
	payload := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner"}},"results":[{"ruleId":"R","message":{"text":"finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":` + quote(uri) + `}}}]}]}]}`
	got, err := ParseSARIF([]byte(payload), root)
	if err != nil { t.Fatal(err) }
	if len(got.Findings) != 1 || got.Findings[0].Path != "src/main.go" { t.Fatalf("absolute in-root location not normalized: %+v", got.Findings) }
}

func TestParseSARIFFallbackFingerprintIsStable(t *testing.T) {
	data := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"scanner","version":"1"}},"results":[{"ruleId":"R","message":{"text":"finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"x.go"},"region":{"startLine":9}}}]}]}]}`)
	first, err := ParseSARIF(data, t.TempDir()); if err != nil { t.Fatal(err) }
	second, err := ParseSARIF(data, t.TempDir()); if err != nil { t.Fatal(err) }
	if first.Findings[0].Fingerprint == "" || first.Findings[0].Fingerprint != second.Findings[0].Fingerprint {
		t.Fatalf("fingerprint is not stable: %q vs %q", first.Findings[0].Fingerprint, second.Findings[0].Fingerprint)
	}
}

func TestHashFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte("rules: []\n"), 0600); err != nil { t.Fatal(err) }
	got, err := HashFile(p)
	if err != nil || len(got) != 64 { t.Fatalf("HashFile = %q, %v", got, err) }
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
