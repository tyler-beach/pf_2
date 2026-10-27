package obligations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
)

func TestBuildIndexCreatesReviewOnlyCandidatesWithExactSourceLines(t *testing.T) {
	root := t.TempDir()
	source := strings.Join([]string{
		"# ADR-901: Service timeout policy", "",
		"## Status", "", "Accepted", "",
		"## Context", "", "Unbounded network calls can exhaust resources.", "",
		"## Decision", "",
		"Every outbound network call must define a finite timeout.",
		"Service clients should apply bounded retries with jitter.",
		"An exception may be approved only when a documented interoperability constraint prevents a timeout.",
		"This observation is descriptive, not normative.", "",
		"## Consequences", "", "Implementations must expose timeout values in configuration.", "",
		"## Example", "", "~~~go",
		"// must not be extracted from a code fence",
		"~~~", "",
	}, "\n")
	path := filepath.Join(root, "ADR-901-timeouts.md")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil { t.Fatal(err) }
	doc, err := adringest.Parse("ADR-901-timeouts.md", []byte(source))
	if err != nil { t.Fatal(err) }
	rawIndex := []byte("{\"schema_version\":\"1\",\"source_root\":\"adrs\"}")
	got, err := BuildIndex([]adringest.Document{doc}, root, "adrs", rawIndex)
	if err != nil { t.Fatal(err) }
	if got.SchemaVersion != "1" || got.DocumentCount != 1 || got.DocumentsWithCandidates != 1 || got.CandidateCount != 4 {
		t.Fatalf("unexpected candidate index summary: %+v", got)
	}
	if len(got.SourceIndexSHA256) != 64 { t.Fatalf("missing index provenance: %+v", got) }
	markers := map[string]bool{"Every outbound": false, "Service clients": false, "An exception": false, "Implementations": false}
	lines := strings.Split(source, "\n")
	for _, candidate := range got.Candidates {
		if candidate.ID == "" || !strings.HasPrefix(candidate.ID, "CAND-") { t.Errorf("missing stable candidate ID: %+v", candidate) }
		if candidate.SourceSHA256 != doc.SourceSHA256 || candidate.SourcePath != filepath.ToSlash(filepath.Join("adrs", doc.SourcePath)) {
			t.Errorf("source provenance lost: %+v", candidate)
		}
		if candidate.SourceLineStart != candidate.SourceLineEnd || candidate.SourceLineStart < 1 || candidate.SourceLineStart > len(lines) {
			t.Errorf("candidate must identify an exact source line: %+v", candidate)
		}
		if candidate.ApprovalStatus != "pending_review" || candidate.EnforcementMode != "review_required" || candidate.Executable {
			t.Errorf("candidate was accidentally made enforceable: %+v", candidate)
		}
		if strings.Contains(candidate.Statement, "must not be extracted") { t.Errorf("code fence emitted a candidate: %+v", candidate) }
		if !strings.Contains(lines[candidate.SourceLineStart-1], candidate.Statement) {
			t.Errorf("source line %d does not contain the candidate text %q", candidate.SourceLineStart, candidate.Statement)
		}
		for marker := range markers { if strings.Contains(candidate.Statement, marker) { markers[marker] = true } }
	}
	for marker, found := range markers { if !found { t.Errorf("candidate not found for %q", marker) } }
	var exceptionFound bool
	for _, c := range got.Candidates { if c.Kind == "exception_candidate" && c.Modality == "may" { exceptionFound = true } }
	if !exceptionFound { t.Fatal("exception clause was not marked for review") }
}

func TestBuildIndexIsDeterministic(t *testing.T) {
	root := t.TempDir()
	source := "# ADR-001: Timeout\n\n## Decision\n\nThe service must set a timeout.\n"
	path := filepath.Join(root, "ADR-001.md")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil { t.Fatal(err) }
	doc, err := adringest.Parse("ADR-001.md", []byte(source))
	if err != nil { t.Fatal(err) }
	raw := []byte("{\"schema_version\":\"1\"}")
	first, err := BuildIndex([]adringest.Document{doc}, root, ".", raw)
	if err != nil { t.Fatal(err) }
	second, err := BuildIndex([]adringest.Document{doc}, root, ".", raw)
	if err != nil { t.Fatal(err) }
	a, err := json.Marshal(first); if err != nil { t.Fatal(err) }
	b, err := json.Marshal(second); if err != nil { t.Fatal(err) }
	if string(a) != string(b) { t.Fatalf("candidate generation is not deterministic:\n%s\n%s", a, b) }
}

func TestBuildIndexRejectsSourceDriftAndUnsafePaths(t *testing.T) {
	root := t.TempDir()
	source := "# ADR-001: Timeout\n\n## Decision\n\nThe service must set a timeout.\n"
	path := filepath.Join(root, "ADR-001.md")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil { t.Fatal(err) }
	doc, err := adringest.Parse("ADR-001.md", []byte(source))
	if err != nil { t.Fatal(err) }
	raw := []byte("{\"schema_version\":\"1\"}")
	if err := os.WriteFile(path, []byte(source+"Source drift.\n"), 0600); err != nil { t.Fatal(err) }
	if _, err := BuildIndex([]adringest.Document{doc}, root, ".", raw); err == nil || !strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("stale source index should fail closed, got %v", err)
	}
	doc.SourcePath = "../outside.md"
	if _, err := BuildIndex([]adringest.Document{doc}, root, ".", raw); err == nil || !strings.Contains(err.Error(), "safe repository-relative") {
		t.Fatalf("path traversal should fail closed, got %v", err)
	}
}

func TestBuildIndexAcceptsNoNormativeStatements(t *testing.T) {
	root := t.TempDir()
	source := "# ADR-902: Background\n\n## Context\n\nTeams are growing and the current system is difficult to maintain.\n"
	path := filepath.Join(root, "ADR-902.md")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil { t.Fatal(err) }
	doc, err := adringest.Parse("ADR-902.md", []byte(source))
	if err != nil { t.Fatal(err) }
	got, err := BuildIndex([]adringest.Document{doc}, root, ".", []byte("{}"))
	if err != nil { t.Fatal(err) }
	if got.DocumentCount != 1 || got.CandidateCount != 0 || len(got.Candidates) != 0 {
		t.Fatalf("non-normative rationale should produce no candidates: %+v", got)
	}
}

func TestBuildIndexRejectsSymlinkedSource(t *testing.T) {
	root := t.TempDir()
	source := "# ADR-001: Timeout\n\n## Decision\n\nService must set a timeout.\n"
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte(source), 0600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(root, "ADR-001.md")); err != nil { t.Skipf("symlink unavailable: %v", err) }
	doc, err := adringest.Parse("ADR-001.md", []byte(source))
	if err != nil { t.Fatal(err) }
	_, err = BuildIndex([]adringest.Document{doc}, root, ".", []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), "uses a symlinked path") { t.Fatalf("expected symlink rejection, got %v", err) }
}


// TestSpecificationFixtureRemainsReviewOnly exercises ordinary technical prose,
// including numeric bounds, compound conditions, and an exception clause.
func TestSpecificationFixtureRemainsReviewOnly(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "specifications", "service-runtime.md")
	source, err := os.ReadFile(fixturePath)
	if err != nil { t.Fatal(err) }
	doc, err := adringest.Parse("service-runtime.md", source)
	if err != nil { t.Fatal(err) }
	candidates, err := ExtractDocument(doc, "testdata/specifications/service-runtime.md", source)
	if err != nil { t.Fatal(err) }

	var timeout, retries, health, exception bool
	for _, candidate := range candidates {
		if candidate.ApprovalStatus != "pending_review" || candidate.EnforcementMode != "review_required" || candidate.Executable {
			t.Errorf("specification extraction must not activate enforcement: %+v", candidate)
		}
		if candidate.SourceLineStart != candidate.SourceLineEnd || candidate.SourceLineStart < 1 {
			t.Errorf("candidate must preserve an exact source line: %+v", candidate)
		}
		for _, hint := range candidate.ReviewHints {
			switch hint {
			case "condition_or_exception", "compound_or_conjoined", "conditional_scope", "subjective_or_underspecified":
			default:
				t.Errorf("unexpected review hint %q", hint)
			}
		}
		if strings.Contains(candidate.Statement, "this is not evidence") {
			t.Errorf("fenced illustrative code was extracted as an obligation: %+v", candidate)
		}
		switch {
		case strings.Contains(candidate.Statement, "timeout no greater than 30 seconds"):
			timeout = candidate.Modality == "must" && candidate.PatternHint == "configuration.bounds_or_required_values" && hasReviewHint(candidate, "compound_or_conjoined")
		case strings.Contains(candidate.Statement, "limited to three attempts"):
			retries = candidate.Modality == "must" && candidate.PatternHint == "configuration.bounds_or_required_values"
		case strings.Contains(candidate.Statement, "expose a health endpoint"):
			health = candidate.Modality == "must"
		case strings.Contains(candidate.Statement, "exception may be approved only when"):
			exception = candidate.Modality == "may" && candidate.Kind == "exception_candidate" && hasReviewHint(candidate, "condition_or_exception") && hasReviewHint(candidate, "conditional_scope")
		}
	}
	if !timeout || !retries || !health || !exception {
		t.Fatalf("expected timeout, retry, health, and exception candidates; got timeout=%v retries=%v health=%v exception=%v candidates=%+v", timeout, retries, health, exception, candidates)
	}
}


func TestDetectModalityUsesFirstCueAndHandlesNegation(t *testing.T) {
	tests := []struct { statement, want string }{
		{"The service must not execute shell commands.", "must_not"},
		{"A timeout is not required for local in-memory operations.", "not_required"},
		{"An exception may be approved only when a documented constraint requires it.", "may"},
		{"Retries should be bounded and must use jitter.", "should"},
		{"The service must set a finite timeout.", "must"},
		{"The deployment may use a sidecar.", "may"},
	}
	for _, tc := range tests {
		got, ok := detectModality(tc.statement)
		if !ok || got != tc.want {
			t.Errorf("detectModality(%q) = %q, %v; want %q, true", tc.statement, got, ok, tc.want)
		}
	}
	if got, ok := detectModality("The mustache service runs normally."); ok || got != "" {
		t.Errorf("non-normative word containing 'must' should not be classified, got %q, %v", got, ok)
	}
}
