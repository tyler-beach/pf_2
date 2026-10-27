package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
)

func TestRunIngestADRsWritesDeterministicIndex(t *testing.T) {
	root := t.TempDir()
	adrRoot := filepath.Join(root, "adrs")
	if err := os.MkdirAll(adrRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	source := "# ADR-001: Use explicit timeouts\n\n## Status\n\nAccepted\n\n## Decision\n\nSet a timeout on every outbound request.\n"
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "generated", "index.json")
	if err := runIngestADRs([]string{"--root", adrRoot, "--json-out", output}); err != nil {
		t.Fatalf("runIngestADRs() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var index ingestionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("invalid output JSON: %v", err)
	}
	if index.SchemaVersion != "1" || len(index.Documents) != 1 {
		t.Fatalf("unexpected index: %+v", index)
	}
	if index.Documents[0].ID != "ADR-001" || index.Documents[0].DecisionText == "" {
		t.Fatalf("document was not ingested: %+v", index.Documents[0])
	}
	if !strings.Contains(string(data), "source_sha256") {
		t.Fatal("index should preserve source hash provenance")
	}
}

func TestRunIngestADRsRejectsDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"ADR-001.md", "another-copy.md"} {
		source := "# ADR-001: Duplicate ID\n\n## Decision\n\nUse explicit timeouts.\n"
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := runIngestADRs([]string{"--root", root, "--json-out", filepath.Join(t.TempDir(), "index.json")})
	if err == nil || !strings.Contains(err.Error(), "duplicate ADR ID") {
		t.Fatalf("expected duplicate ADR ID error, got %v", err)
	}
}

func TestRunIngestADRsRejectsMissingRoot(t *testing.T) {
	err := runIngestADRs([]string{"--root", filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected missing root error")
	}
}

func TestIngestionIndexDocumentsHaveProvenance(t *testing.T) {
	doc, err := adringest.Parse("ADR-9.md", []byte("# ADR-9: Example\n\n## Decision\n\nDo the thing.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.SourceSHA256) != 64 || doc.Sections[0].StartLine != 3 {
		t.Fatalf("unexpected provenance: %+v", doc)
	}
}


func TestRunAssessRepoConsumesAnalyzerEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0700); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "cmd", "main.go"), []byte("package main\n"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "rules", "security.yml"), []byte("rules: []\n"), 0600); err != nil { t.Fatal(err) }
	catalog := assessment.Catalog{SchemaVersion: "1", ADRs: []assessment.ADR{{
		ID: "ADR-003", Title: "Analyzer policy", Version: 1, Status: "accepted",
		Requirements: []assessment.Requirement{{
			ID: "REQ-003-01", Title: "No shell execution", Severity: "error",
			Verifier: assessment.Verifier{Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"rule.no-shell"}, SourcePathPrefixes: []string{"cmd/"}},
		}},
	}}}
	catalogPath := filepath.Join(root, "requirements.json")
	catalogBytes, err := json.Marshal(catalog)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(catalogPath, catalogBytes, 0600); err != nil { t.Fatal(err) }

	raw := []byte("{\\"version\\":\\"2.1.0\\",\\"runs\\":[{\\"tool\\":{\\"driver\\":{\\"name\\":\\"opengrep\\",\\"version\\":\\"1.30.0\\"}},\\"results\\":[]}]}")
	evidence, err := analyzer.ParseSARIF(raw, root)
	if err != nil { t.Fatal(err) }
	configHash, err := analyzer.HashFile(filepath.Join(root, "rules", "security.yml"))
	if err != nil { t.Fatal(err) }
	inputHash, err := analyzer.HashInputTree(root)
	if err != nil { t.Fatal(err) }
	evidence.ConfigurationPath = "rules/security.yml"
	evidence.ConfigurationSHA256 = configHash
	evidence.InputSHA256 = inputHash
	evidence.ScannedPaths = []string{"cmd/main.go"}
	evidenceRoot := t.TempDir()
	evidencePath := filepath.Join(evidenceRoot, "opengrep.json")
	evidenceBytes, err := json.Marshal(evidence)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(evidencePath, evidenceBytes, 0600); err != nil { t.Fatal(err) }
	sarifPath := filepath.Join(evidenceRoot, "opengrep.sarif")
	if err := os.WriteFile(sarifPath, raw, 0600); err != nil { t.Fatal(err) }
	resultPath := filepath.Join(root, "assessment.json")
	if err := runAssess([]string{"--root", root, "--requirements", catalogPath,
		"--report", filepath.Join(root, "report.md"), "--json-out", resultPath,
		"--sarif-out", filepath.Join(root, "results.sarif"), "--analyzer-evidence", evidencePath,
		"--analyzer-sarif", sarifPath}, true); err != nil {
		t.Fatalf("runAssess() error = %v", err)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil { t.Fatal(err) }
	var got assessment.Result
	if err := json.Unmarshal(data, &got); err != nil { t.Fatal(err) }
	if got.Verdict != "PASS" || len(got.AnalyzerResults) != 1 { t.Fatalf("analyzer evidence was not included: %+v", got) }
	if len(got.AnalyzerResults[0].ScannedPaths) == 0 { t.Fatalf("verified scan inventory missing: %+v", got.AnalyzerResults[0]) }
}

func TestRunAssessRejectsNormalizedEvidenceWithoutRawSARIF(t *testing.T) {
	root := t.TempDir()
	catalogPath := filepath.Join(root, "requirements.json")
	if err := os.WriteFile(catalogPath, []byte("{\\"schema_version\\":\\"1\\",\\"adrs\\":[]}"), 0600); err != nil { t.Fatal(err) }
	evidencePath := filepath.Join(root, "evidence.json")
	if err := os.WriteFile(evidencePath, []byte("{\\"schema_version\\":\\"1\\",\\"sarif_version\\":\\"2.1.0\\",\\"status\\":\\"COMPLETE\\"}"), 0600); err != nil { t.Fatal(err) }
	err := runAssess([]string{"--root", root, "--requirements", catalogPath,
		"--input", filepath.Join(root, "issue.md"), "--analyzer-evidence", evidencePath}, false)
	if err == nil || !strings.Contains(err.Error(), "repository assessments") {
		t.Fatalf("expected submission-mode evidence rejection, got %v", err)
	}
}
