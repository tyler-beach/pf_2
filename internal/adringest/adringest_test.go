package adringest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMADRDocumentPreservesSectionsAndProvenance(t *testing.T) {
	source := []byte(`# ADR-012: Prefer asynchronous messaging

## Status

Accepted

## Context

Services have become tightly coupled.

## Decision

Prefer asynchronous messaging between independently deployed services.

## Consequences

Consumers must handle retries and duplicate messages.

`)
	doc, err := Parse("adrs/ADR-012-messaging.md", source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.ID != "ADR-012" {
		t.Errorf("ID = %q, want ADR-012", doc.ID)
	}
	if doc.Title != "ADR-012: Prefer asynchronous messaging" {
		t.Errorf("Title = %q", doc.Title)
	}
	if doc.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", doc.Status)
	}
	if doc.SourcePath != "adrs/ADR-012-messaging.md" {
		t.Errorf("SourcePath = %q", doc.SourcePath)
	}
	if len(doc.SourceSHA256) != 64 {
		t.Errorf("SHA-256 should be 64 hex chars, got %q", doc.SourceSHA256)
	}
	if len(doc.Sections) != 4 {
		t.Fatalf("got %d sections, want 4", len(doc.Sections))
	}
	decision := sectionByHeading(t, doc, "Decision")
	if decision.StartLine != 11 {
		t.Errorf("Decision.StartLine = %d, want 14", decision.StartLine)
	}
	if !strings.Contains(decision.Text, "Prefer asynchronous messaging") {
		t.Errorf("Decision text missing expected content: %q", decision.Text)
	}
	if doc.DecisionText != decision.Text {
		t.Errorf("DecisionText should match Decision section")
	}
}

func TestParseLegacyADRUsesFilenameAsStableFallbackID(t *testing.T) {
	source := []byte(`# Use queues for cross-service communication

Status: accepted

## Why

The services should not share a synchronous runtime dependency.

## Decision

Use a durable queue for cross-service work.
`)
	doc, err := Parse("docs/0007-use-queues.md", source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.ID != "0007-use-queues" {
		t.Errorf("ID = %q, want filename fallback", doc.ID)
	}
	if doc.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", doc.Status)
	}
	if len(doc.Sections) != 2 {
		t.Errorf("got %d sections, want 2", len(doc.Sections))
	}
}

func TestParseIgnoresHeadingsInsideCodeFences(t *testing.T) {
	source := []byte("# ADR-004: Keep examples literal\n\n## Decision\n\nUse typed APIs.\n\n```markdown\n# Not an ADR heading\n## Decision\nignore me\n```\n")
	doc, err := Parse("ADR-004.md", source)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(doc.Sections) != 1 || doc.Sections[0].Heading != "Decision" {
		t.Fatalf("code fence incorrectly became a section: %+v", doc.Sections)
	}
}

func TestParseRejectsEmptyOrHeadinglessDocuments(t *testing.T) {
	for name, source := range map[string][]byte{
		"empty": nil,
		"blank": []byte(" \n\n"),
		"no-heading": []byte("Status: accepted\nSome text without a title.\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("adr.md", source); err == nil {
				t.Fatal("Parse() expected an error")
			}
		})
	}
}

func TestParseDirSortsDocumentsAndSkipsNonMarkdown(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"ADR-002.md": "# ADR-002: Second\n\n## Decision\n\nSecond decision.\n",
		"ADR-001.md": "# ADR-001: First\n\n## Decision\n\nFirst decision.\n",
		"notes.txt": "not markdown",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	docs, err := ParseDir(root)
	if err != nil {
		t.Fatalf("ParseDir() error = %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
	if docs[0].ID != "ADR-001" || docs[1].ID != "ADR-002" {
		t.Errorf("documents are not sorted: %q, %q", docs[0].ID, docs[1].ID)
	}
}

func TestParseDirReportsMalformedMarkdownFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.md"), []byte("no heading"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDir(root); err == nil {
		t.Fatal("ParseDir() expected malformed document error")
	}
}

func sectionByHeading(t *testing.T, doc Document, heading string) Section {
	t.Helper()
	for _, section := range doc.Sections {
		if section.Heading == heading {
			return section
		}
	}
	t.Fatalf("section %q not found in %+v", heading, doc.Sections)
	return Section{}
}


func FuzzParseADRMarkdown(f *testing.F) {
	f.Add([]byte("# ADR-001: Explicit timeouts\n\n## Status\n\nAccepted\n\n## Decision\n\nSet a timeout.\n"))
	f.Add([]byte("# Legacy decision\n\nSome prose with [a link](https://example.com).\n"))
	f.Add([]byte("# ADR-002: Fence safety\n\n```md\n# not a heading\n```\n"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 { t.Skip() }
		doc, err := Parse("fuzz.md", data)
		if err != nil { return }
		if doc.SourcePath != "fuzz.md" { t.Fatalf("unexpected source path %q", doc.SourcePath) }
		if len(doc.SourceSHA256) != 64 { t.Fatalf("source SHA-256 missing: %q", doc.SourceSHA256) }
		if doc.ID == "" { t.Fatal("successfully parsed document has no stable ID") }
	})
}
