package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
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
