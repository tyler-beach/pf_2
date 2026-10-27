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
			case "condition_or_exception", "compound_or_conjoined", "conditional_scope", "subjective_or_underspecified", "non_decision_section",
				"ambiguous_interpretation", "attempts_vs_retries_clarification", "conflicting_modality_cues", "split_from_compound":
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


func TestContextualNormativeTextRequiresExtraReview(t *testing.T) {
	source := []byte("# ADR-901: Timeout policy\n\n## Status\n\nAccepted\n\n## Context\n\nLegacy clients must use the old timeout setting.\n\n## Decision\n\nThe service must use a finite request timeout.\n")
	doc, err := adringest.Parse("ADR-901.md", source)
	if err != nil { t.Fatal(err) }
	candidates, err := ExtractDocument(doc, "adrs/ADR-901.md", source)
	if err != nil { t.Fatal(err) }
	if len(candidates) != 2 { t.Fatalf("expected two normative candidates, got %+v", candidates) }
	for _, candidate := range candidates {
		if candidate.SectionHeading == "Context" && !hasReviewHint(candidate, "non_decision_section") {
			t.Errorf("contextual normative text must be flagged for review: %+v", candidate)
		}
		if candidate.SectionHeading == "Decision" && hasReviewHint(candidate, "non_decision_section") {
			t.Errorf("decision text was incorrectly marked contextual: %+v", candidate)
		}
	}
}

func hasReviewHint(candidate Candidate, wanted string) bool {
	for _, hint := range candidate.ReviewHints {
		if hint == wanted { return true }
	}
	return false
}

func TestBuildIndexSplitsMultipleNormativeSentencesInPlainEnglishSpecification(t *testing.T) {
	root := t.TempDir()
	source := strings.Join([]string{
		"# Service runtime requirements",
		"",
		"## Requirements",
		"",
		"All outbound HTTP clients must set a finite request timeout no greater than 30 seconds. Automatic retries must be limited to three attempts. Each service must expose a health endpoint. Exceptions may be approved only when an approved, documented interoperability constraint requires it.",
	}, "\n")
	path := filepath.Join(root, "service-runtime.md")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("service-runtime.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != "service-runtime" {
		t.Fatalf("free-form specification should use stable filename ID, got %q", doc.ID)
	}

	index, err := BuildIndex(
		[]adringest.Document{doc},
		root,
		"specifications",
		[]byte("{\"schema_version\":\"1\",\"source_root\":\"specifications\"}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if index.CandidateCount != 4 {
		t.Fatalf("got %d candidates; want one per normative sentence (4): %+v", index.CandidateCount, index.Candidates)
	}

	wantStatements := []string{
		"All outbound HTTP clients must set a finite request timeout no greater than 30 seconds.",
		"Automatic retries must be limited to three attempts.",
		"Each service must expose a health endpoint.",
		"Exceptions may be approved only when an approved, documented interoperability constraint requires it.",
	}
	gotStatements := make(map[string]bool, len(index.Candidates))
	ids := make(map[string]bool, len(index.Candidates))
	lines := strings.Split(source, "\n")
	for _, candidate := range index.Candidates {
		gotStatements[candidate.Statement] = true
		if ids[candidate.ID] {
			t.Errorf("duplicate candidate ID %q", candidate.ID)
		}
		ids[candidate.ID] = true
		if candidate.SourceLineStart != 5 || candidate.SourceLineEnd != 5 {
			t.Errorf("candidate should retain exact source line 5, got %d-%d", candidate.SourceLineStart, candidate.SourceLineEnd)
		}
		if !strings.Contains(lines[candidate.SourceLineStart-1], candidate.Statement) {
			t.Errorf("candidate statement %q is not present on cited source line", candidate.Statement)
		}
		if candidate.ApprovalStatus != "pending_review" || candidate.Executable {
			t.Errorf("plain-English extraction must remain review-only: %+v", candidate)
		}
	}
	for _, statement := range wantStatements {
		if !gotStatements[statement] {
			t.Errorf("missing standalone candidate sentence %q", statement)
		}
	}
}

func TestBuildIndexRejectsOversizedSourceLineAfterSentenceSplitting(t *testing.T) {
	root := t.TempDir()
	longLine := strings.Repeat("Every client must set a finite timeout. ", maxCandidateLineBytes/20)
	source := "# ADR-903: Long requirement line\n\n## Requirements\n\n" + longLine + "\n"
	path := filepath.Join(root, "ADR-903.md")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-903.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildIndex(
		[]adringest.Document{doc},
		root,
		"adrs",
		[]byte("{\"schema_version\":\"1\",\"source_root\":\"adrs\"}"),
	)
	if err == nil || !strings.Contains(err.Error(), "candidate statement limit") {
		t.Fatalf("oversized normative source line should be rejected before it expands into many candidates, got %v", err)
	}
}

func TestSplitSentencesPreservesAbbreviationsAndDecimals(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "country abbreviation",
			line: "Clients must follow U.S. policy. Every service must expose a health endpoint.",
			want: []string{
				"Clients must follow U.S. policy.",
				"Every service must expose a health endpoint.",
			},
		},
		{
			name: "decimal value",
			line: "Clients must set a timeout of 3.5 seconds. Every service must expose a health endpoint.",
			want: []string{
				"Clients must set a timeout of 3.5 seconds.",
				"Every service must expose a health endpoint.",
			},
		},
		{
			name: "example abbreviation",
			line: "Clients must use e.g. a shared HTTP client. Every service must expose a health endpoint.",
			want: []string{
				"Clients must use e.g. a shared HTTP client.",
				"Every service must expose a health endpoint.",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSentences(tc.line)
			if len(got) != len(tc.want) {
				t.Fatalf("splitSentences() produced %d fragments, want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("fragment %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestBuildIndexExtractsNormativePreambleWithoutSubheadings(t *testing.T) {
	root := t.TempDir()
	source := "# Runtime requirements\n\nEvery service must expose a health endpoint.\n"
	path := filepath.Join(root, "runtime.md")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("runtime.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 0 {
		t.Fatalf("fixture should exercise a title-only document, got sections: %+v", doc.Sections)
	}
	index, err := BuildIndex(
		[]adringest.Document{doc},
		root,
		"specifications",
		[]byte("{\"schema_version\":\"1\",\"source_root\":\"specifications\"}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if index.CandidateCount != 1 {
		t.Fatalf("title-only specification lost normative preamble text: %+v", index)
	}
	candidate := index.Candidates[0]
	if candidate.Statement != "Every service must expose a health endpoint." {
		t.Fatalf("unexpected extracted preamble statement: %+v", candidate)
	}
	if candidate.SectionHeading != "Preamble" || candidate.SourceLineStart != 3 || candidate.SourceLineEnd != 3 {
		t.Fatalf("preamble must preserve source line provenance: %+v", candidate)
	}
	if !hasReviewHint(candidate, "non_decision_section") {
		t.Fatalf("preamble candidate should be flagged for reviewer attention: %+v", candidate)
	}
	if candidate.ApprovalStatus != "pending_review" || candidate.Executable {
		t.Fatalf("preamble extraction must remain review-only: %+v", candidate)
	}
}

func TestBuildIndexExtractsPreambleAlongsideHeadedSections(t *testing.T) {
	root := t.TempDir()
	source := "# Runtime requirements\n\nEvery service must expose a health endpoint.\n\n## Decision\n\nDomain code must not import internal infrastructure packages.\n"
	path := filepath.Join(root, "runtime.md")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("runtime.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	index, err := BuildIndex(
		[]adringest.Document{doc},
		root,
		"specifications",
		[]byte("{\"schema_version\":\"1\",\"source_root\":\"specifications\"}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if index.CandidateCount != 2 {
		t.Fatalf("expected preamble and Decision candidates, got %+v", index.Candidates)
	}
	if index.Candidates[0].SectionHeading != "Preamble" || index.Candidates[0].SourceLineStart != 3 {
		t.Errorf("preamble candidate missing or moved: %+v", index.Candidates[0])
	}
	if index.Candidates[1].SectionHeading != "Decision" || index.Candidates[1].SourceLineStart != 7 {
		t.Errorf("headed-section candidate changed: %+v", index.Candidates[1])
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

func TestExpandNormativeStatementsSplitsCompoundObligations(t *testing.T) {
	line := "Clients must set a finite timeout. Automatic retries must be limited to three attempts."
	parts := expandNormativeStatements(line)
	if len(parts) != 2 {
		t.Fatalf("expected 2 normative fragments, got %d: %#v", len(parts), parts)
	}
	joined := "Timeouts must be finite and retries must be capped."
	parts = expandNormativeStatements(joined)
	if len(parts) != 2 {
		t.Fatalf("expected and-split of dual modalities, got %d: %#v", len(parts), parts)
	}
}

func TestClassifyStatementStructureSurfacesConditionsExceptionsAmbiguity(t *testing.T) {
	conditions, exceptions, ambiguity := classifyStatementStructure(
		"An exception may be approved only when a documented interoperability constraint requires it.")
	if len(conditions) == 0 || len(exceptions) == 0 {
		t.Fatalf("expected condition and exception markers, got conditions=%v exceptions=%v", conditions, exceptions)
	}
	_, _, ambiguity = classifyStatementStructure("Services should prefer async messaging where appropriate.")
	if len(ambiguity) == 0 {
		t.Fatal("expected ambiguity markers for preference language")
	}
}

func TestExtractDocumentRejectsFencedExamplesAndDescriptiveProse(t *testing.T) {
	root := t.TempDir()
	source := strings.Join([]string{
		"# ADR-950: Negative extraction cases", "",
		"## Status", "", "Accepted", "",
		"## Context", "",
		"This section describes historical behavior without obligations.",
		"Services typically used synchronous calls in the past.", "",
		"## Decision", "",
		"Quoted guidance such as \"services must be resilient\" in examples is illustrative only.",
		"```",
		"must execute forbidden fence content",
		"```",
		"Services must not call eval on untrusted input.",
		"Services must set timeouts and services must limit retries.",
	}, "\n")
	path := filepath.Join(root, "ADR-950.md")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-950.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := ExtractDocument(doc, "ADR-950.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if c.Executable {
			t.Fatalf("candidate unexpectedly executable: %+v", c)
		}
		if strings.Contains(c.Statement, "forbidden fence") {
			t.Fatalf("fenced example leaked: %+v", c)
		}
		if strings.Contains(c.Statement, "typically used") {
			t.Fatalf("descriptive prose became a candidate: %+v", c)
		}
	}
	var foundCompound bool
	for _, c := range candidates {
		if strings.Contains(c.Statement, "must set timeouts") || strings.Contains(c.Statement, "must limit retries") {
			foundCompound = true
		}
	}
	if !foundCompound {
		t.Fatalf("expected compound split candidates, got %#v", candidates)
	}
}

func TestExtractDocumentByteIdenticalOnRepeat(t *testing.T) {
	root := t.TempDir()
	source := "# ADR-951: Repeatability\n\n## Decision\n\nClients must set timeouts. Retries must be capped.\n"
	path := filepath.Join(root, "ADR-951.md")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-951.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	first, err := ExtractDocument(doc, "ADR-951.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExtractDocument(doc, "ADR-951.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatalf("re-extract diverged:\n%s\n%s", a, b)
	}
}
