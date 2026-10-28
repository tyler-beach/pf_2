package prscan

import (
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

func TestFormatCommentStableShape(t *testing.T) {
	got := FormatComment(CommentBlocks{
		Severity: "error",
		ADRID:    "ADR-003",
		Title:    "No dynamic shell",
		Why:      "Avoid shell invocation",
		Evidence: "internal/run.go:14",
		Next:     "Harden the call site — https://example.test/#/docs",
	})
	wantLines := []string{
		"[ERROR] ADR-003 · No dynamic shell",
		"Why: Avoid shell invocation",
		"Evidence: `internal/run.go:14`",
		"Next: Harden the call site — https://example.test/#/docs",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Fatalf("missing %q in:\n%s", line, got)
		}
	}
	if strings.Count(got, "\n") != 3 {
		t.Fatalf("expected 4 lines, got %q", got)
	}
}

func TestFormatFindingUsesSharedShape(t *testing.T) {
	req := assessment.RequirementResult{
		ADR: "ADR-003", RequirementID: "REQ-003-01",
		Title: "No dynamic shell", Status: "FAIL", Severity: "warning",
		Reason: "fallback reason",
	}
	f := assessment.Finding{
		RuleID: "pathfinder.go-shell-command", Path: "internal/run.go", Line: 14,
		Message: "Avoid shell invocation",
	}
	got := FormatFinding(req, f, "https://example.test/#/docs/repo-ci")
	if !strings.HasPrefix(got, "[WARNING] ADR-003 · No dynamic shell\n") {
		t.Fatalf("unexpected prefix:\n%s", got)
	}
	if !strings.Contains(got, "Next: Fix deviation for REQ-003-01 — https://example.test/#/docs/repo-ci") {
		t.Fatalf("missing next:\n%s", got)
	}
}

func TestMapSeverityPrefersRequirementOverAnalyzer(t *testing.T) {
	if got := mapSeverity("error", "warning"); got != "ERROR" {
		t.Fatalf("req error + analyzer warning → %q", got)
	}
	if got := mapSeverity("warning", "error"); got != "WARNING" {
		t.Fatalf("req warning wins over analyzer error → %q", got)
	}
	if got := mapSeverity("", "warning"); got != "WARNING" {
		t.Fatalf("analyzer fallback → %q", got)
	}
	if got := mapSeverity("", ""); got != "ERROR" {
		t.Fatalf("default → %q", got)
	}
}

func TestFormatSummaryOmitsNAList(t *testing.T) {
	got := FormatSummary(Summary{
		Verdict: "FAIL", CoverageStatus: "PARTIAL", Applicable: 12, RequirementCount: 200,
		Posted: 3, CappedFrom: 40, NotApplicable: 180, Pass: 5, OtherNonFail: 2,
		Deduped: 1, PRURL: "https://github.com/acme/widgets/pull/1",
		HeadSHA: "abcdef0123456789", Marker: "abc123",
		RunID: "123456789",
	})
	if strings.Contains(got, "ADR-") && strings.Contains(got, "NOT_APPLICABLE requirement") {
		t.Fatal("summary should not enumerate N/A requirements")
	}
	if !strings.Contains(got, "180 NOT_APPLICABLE") {
		t.Fatalf("expected N/A count rollup:\n%s", got)
	}
	if !strings.Contains(got, "<!-- pathfinder-pr-scan:abc123 -->") {
		t.Fatalf("missing dedupe marker:\n%s", got)
	}
	if !strings.Contains(got, "#/runs?run_id=123456789") || !strings.Contains(got, "#/submissions") {
		t.Fatalf("expected workshop deep links:\n%s", got)
	}
}
