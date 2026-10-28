package prscan

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

func sampleResult() assessment.Result {
	return assessment.Result{
		SchemaVersion:              "1",
		Verdict:                    "FAIL",
		CoverageStatus:             "PARTIAL",
		EvidenceMode:               string(assessment.EvidenceModeRepository),
		RequirementCount:           5,
		ApplicableRequirementCount: 2,
		Requirements: []assessment.RequirementResult{
			{
				ADR: "ADR-NA", RequirementID: "REQ-NA", Title: "N/A thing",
				Status: "NOT_APPLICABLE", Severity: "info",
				Findings: []assessment.Finding{{Path: "x.go", Line: 1, Message: "should never post"}},
			},
			{
				ADR: "ADR-PASS", RequirementID: "REQ-PASS", Title: "Pass thing",
				Status: "PASS", Severity: "error",
			},
			{
				ADR: "ADR-003", RequirementID: "REQ-003-01", Title: "No dynamic shell",
				Status: "FAIL", Severity: "error",
				Findings: []assessment.Finding{{
					RuleID: "pathfinder.go-shell-command", Path: "internal/run.go", Line: 14,
					Column: 5, Message: "Avoid shell invocation", Fingerprint: "fp-1",
				}},
			},
			{
				ADR: "ADR-003", RequirementID: "REQ-003-01", Title: "No dynamic shell",
				Status: "FAIL", Severity: "error",
				Findings: []assessment.Finding{{
					RuleID: "pathfinder.go-shell-command", Path: "internal/run.go", Line: 14,
					Message: "duplicate", Fingerprint: "fp-1",
				}},
			},
			{
				ADR: "ADR-004", RequirementID: "REQ-004-01", Title: "Warn finding",
				Status: "FAIL", Severity: "warning",
				Findings: []assessment.Finding{{
					RuleID: "rule-warn", Path: "pkg/a.go", Line: 3, Message: "warn msg",
				}},
			},
			{
				ADR: "ADR-005", RequirementID: "REQ-005-01", Title: "Fail without path",
				Status: "FAIL", Severity: "error",
				Findings: []assessment.Finding{{Message: "no path"}},
			},
		},
	}
}

func TestConvertAssessmentNoNAFloodAndDedupe(t *testing.T) {
	conv := ConvertAssessment(sampleResult(), ConvertOptions{MaxInline: 10, DocsBase: "https://example.test/#/docs"})
	if len(conv.RDJSON.Diagnostics) != 2 {
		t.Fatalf("want 2 diagnostics, got %d: %+v", len(conv.RDJSON.Diagnostics), conv.RDJSON.Diagnostics)
	}
	for _, d := range conv.RDJSON.Diagnostics {
		if strings.Contains(d.Message, "should never post") {
			t.Fatal("N/A finding leaked into rdjson")
		}
		if !strings.Contains(d.Message, "Why:") || !strings.Contains(d.Message, "Evidence:") || !strings.Contains(d.Message, "Next:") {
			t.Fatalf("diagnostic not using shared formatter:\n%s", d.Message)
		}
	}
	if conv.Summary.NotApplicable != 1 || conv.Summary.Pass != 1 {
		t.Fatalf("summary counts wrong: %+v", conv.Summary)
	}
	if conv.Summary.Deduped != 1 {
		t.Fatalf("expected 1 dedupe, got %d", conv.Summary.Deduped)
	}
	if conv.Summary.Posted != 2 || conv.Summary.CappedFrom < 2 {
		t.Fatalf("posted/capped stats: %+v", conv.Summary)
	}

	raw, err := MarshalRDJSON(conv.RDJSON)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RDJSON
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Source.Name != "pathfinder" {
		t.Fatalf("source: %+v", decoded.Source)
	}
	if decoded.Diagnostics[0].Location.Range == nil || decoded.Diagnostics[0].Location.Range.Start.Line != 14 {
		t.Fatalf("line mapping lost: %+v", decoded.Diagnostics[0].Location)
	}
}

func TestConvertAssessmentCapsInline(t *testing.T) {
	reqs := make([]assessment.RequirementResult, 0, 40)
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("REQ-%03d", i)
		reqs = append(reqs, assessment.RequirementResult{
			ADR: "ADR-X", RequirementID: id, Title: "T", Status: "FAIL", Severity: "error",
			Findings: []assessment.Finding{{
				RuleID: "r", Path: "f.go", Line: i + 1, Message: "m", Fingerprint: "fp-" + id,
			}},
		})
	}
	result := assessment.Result{Verdict: "FAIL", Requirements: reqs, RequirementCount: 40, ApplicableRequirementCount: 40}
	conv := ConvertAssessment(result, ConvertOptions{MaxInline: 5})
	if len(conv.RDJSON.Diagnostics) != 5 {
		t.Fatalf("cap failed: %d", len(conv.RDJSON.Diagnostics))
	}
	if conv.Summary.CappedFrom != 40 || conv.Summary.Posted != 5 {
		t.Fatalf("cap summary: %+v", conv.Summary)
	}
}

func TestNormalizeReporters(t *testing.T) {
	got, err := NormalizeReporters([]string{"github-pr-review", "local", "github-pr-review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("dedupe failed: %v", got)
	}
	if _, err := NormalizeReporters([]string{"doghouse"}); err == nil {
		t.Fatal("expected unsupported reporter error")
	}
	if _, err := NormalizeReporters(nil); err == nil {
		t.Fatal("expected empty error")
	}
}
