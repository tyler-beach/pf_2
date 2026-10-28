package assessment

import "testing"

func TestParseSubmissionKind(t *testing.T) {
	cases := []struct {
		in   string
		want SubmissionKind
	}{
		{"", SubmissionKindUnspecified},
		{"unspecified", SubmissionKindUnspecified},
		{"tech_spec", SubmissionKindTechSpec},
		{"tech-spec", SubmissionKindTechSpec},
		{"code_snippet", SubmissionKindCodeSnippet},
		{"iac", SubmissionKindCodeSnippet},
	}
	for _, tc := range cases {
		got, err := ParseSubmissionKind(tc.in)
		if err != nil {
			t.Fatalf("ParseSubmissionKind(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseSubmissionKind(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if _, err := ParseSubmissionKind("mixed"); err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestClassifyIssueSubmissionKind(t *testing.T) {
	snippetOnly := "### File: pdb.yaml\n```yaml\napiVersion: policy/v1\nkind: PodDisruptionBudget\n```\n"
	if got := ClassifyIssueSubmissionKind(snippetOnly); got != SubmissionKindCodeSnippet {
		t.Fatalf("fence-only → code_snippet, got %q", got)
	}

	techSpec := "# ADR-2100: Pod Disruption Budgets\n\n## Status\nAccepted\n\n## Decision\nCritical Deployments MUST declare a PodDisruptionBudget.\n"
	if got := ClassifyIssueSubmissionKind(techSpec); got != SubmissionKindTechSpec {
		t.Fatalf("ADR prose → tech_spec, got %q", got)
	}

	both := techSpec + "\n" + snippetOnly
	if got := ClassifyIssueSubmissionKind(both); got != SubmissionKindUnspecified {
		t.Fatalf("mixed body → unspecified, got %q", got)
	}

	if got := ClassifyIssueSubmissionKind(""); got != SubmissionKindUnspecified {
		t.Fatalf("empty → unspecified, got %q", got)
	}
}

func TestResolveSubmissionKindPrefersExplicit(t *testing.T) {
	got := ResolveSubmissionKind(SubmissionKindTechSpec, SubmissionKindCodeSnippet)
	if got != SubmissionKindTechSpec {
		t.Fatalf("got %q", got)
	}
	got = ResolveSubmissionKind(SubmissionKindUnspecified, SubmissionKindCodeSnippet)
	if got != SubmissionKindCodeSnippet {
		t.Fatalf("got %q", got)
	}
}

func TestAssessWithOptionsRecordsSubmissionKind(t *testing.T) {
	result := AssessWithOptions(fixtureCatalog(), nil, AssessmentOptions{
		EvidenceMode:   EvidenceModeSubmission,
		SubmissionKind: SubmissionKindCodeSnippet,
	})
	if result.SubmissionKind != string(SubmissionKindCodeSnippet) {
		t.Fatalf("SubmissionKind=%q", result.SubmissionKind)
	}
	empty := AssessWithOptions(fixtureCatalog(), nil, AssessmentOptions{EvidenceMode: EvidenceModeSubmission})
	if empty.SubmissionKind != "" {
		t.Fatalf("unspecified should omit/empty, got %q", empty.SubmissionKind)
	}
}
