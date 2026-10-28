package workshop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyRequirementStatus(t *testing.T) {
	cases := map[string]string{
		"PASS":             AlignmentAligned,
		"pass":             AlignmentAligned,
		"FAIL":             AlignmentDeviated,
		"WARN":             AlignmentDeviated,
		"INDETERMINATE":    AlignmentDeviated,
		"EVIDENCE_MISSING": AlignmentDeviated,
		"ERROR":            AlignmentDeviated,
		"REVIEW_REQUIRED":  AlignmentDeviated,
		"UNSUPPORTED":      AlignmentDeviated,
		"NOT_APPLICABLE":   AlignmentExcluded,
		"":                 AlignmentExcluded,
		"WEIRD":            AlignmentDeviated,
	}
	for status, want := range cases {
		if got := ClassifyRequirementStatus(status); got != want {
			t.Fatalf("%q: got %s want %s", status, got, want)
		}
	}
}

func TestBuildADRFlaggingLeaderboardsAndRates(t *testing.T) {
	report := BuildADRFlagging("run-x", []ADRFlaggingInput{
		{
			RunID: "run-x", SubmissionID: "S1", SubmissionKind: "code_snippet",
			Requirements: []ADRFlaggingInputRequirement{
				{ADRID: "ADR-1", Title: "One", RequirementID: "REQ-1-01", Status: "PASS"},
				{ADRID: "ADR-1", Title: "One", RequirementID: "REQ-1-02", Status: "FAIL", Findings: []ADRFlaggingRule{
					{RuleID: "pathfinder.go-shell", Analyzer: "opengrep"},
				}},
				{ADRID: "ADR-2", Title: "Two", RequirementID: "REQ-2-01", Status: "PASS"},
				{ADRID: "ADR-2", Title: "Two", RequirementID: "REQ-2-02", Status: "PASS"},
				{ADRID: "ADR-3", Title: "Three", RequirementID: "REQ-3-01", Status: "INDETERMINATE"},
				{ADRID: "ADR-3", Title: "Three", RequirementID: "REQ-3-02", Status: "NOT_APPLICABLE"},
			},
		},
		{
			RunID: "run-x", SubmissionID: "S2", SubmissionKind: "tech_spec",
			Requirements: []ADRFlaggingInputRequirement{
				{ADRID: "ADR-1", RequirementID: "REQ-1-01", Status: "FAIL"},
				{ADRID: "ADR-2", RequirementID: "REQ-2-01", Status: "PASS"},
			},
		},
	})
	if report.Summary.ADRTouched != 3 {
		t.Fatalf("touched=%d", report.Summary.ADRTouched)
	}
	if report.Summary.AssessedObs != 7 { // excludes one N/A
		t.Fatalf("assessed obs=%d want 7", report.Summary.AssessedObs)
	}
	if report.Summary.FlagRate == nil || *report.Summary.FlagRate != 3.0/7.0 {
		t.Fatalf("flag rate=%v", report.Summary.FlagRate)
	}
	if len(report.Leaderboards.MostFlagging) == 0 || report.Leaderboards.MostFlagging[0] != "ADR-3" {
		// ADR-3: 1 deviate / 1 assessed = 100%; ADR-1: 2/3
		t.Fatalf("most flagging=%v", report.Leaderboards.MostFlagging)
	}
	if report.Leaderboards.MostFail[0] != "ADR-1" {
		t.Fatalf("most fail=%v", report.Leaderboards.MostFail)
	}
	if report.Leaderboards.MostAligned[0] != "ADR-2" {
		t.Fatalf("most aligned=%v", report.Leaderboards.MostAligned)
	}
	var adr1 ADRFlaggingRow
	for _, r := range report.ADRs {
		if r.ADRID == "ADR-1" {
			adr1 = r
		}
	}
	if adr1.FailCount != 2 || len(adr1.DeviatedRuns) == 0 || len(adr1.Rules) != 1 {
		t.Fatalf("ADR-1 row=%+v", adr1)
	}
	if adr1.ByKind["code_snippet"].Deviated != 1 || adr1.ByKind["tech_spec"].Deviated != 1 {
		t.Fatalf("by kind=%+v", adr1.ByKind)
	}
}

func TestEmitWritesADRFlaggingArtifact(t *testing.T) {
	root := t.TempDir()
	assessment := filepath.Join(root, "assessment.json")
	payload := map[string]any{
		"verdict":           "FAIL",
		"runtime_llm_calls": 0,
		"submission_kind":   "code_snippet",
		"requirements": []map[string]any{
			{"adr_id": "ADR-9", "requirement_id": "REQ-9-01", "title": "Shell", "status": "FAIL",
				"findings": []map[string]any{{"rule_id": "pathfinder.go-shell", "analyzer": "opengrep"}}},
			{"adr_id": "ADR-9", "requirement_id": "REQ-9-02", "title": "Other", "status": "PASS"},
			{"adr_id": "ADR-8", "requirement_id": "REQ-8-01", "title": "NA", "status": "NOT_APPLICABLE"},
		},
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(assessment, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "snap")
	latest, err := Emit(EmitOptions{
		OutDir: out, Run: RunIdentity{RunID: "r1", Workflow: "local"}, AssessmentPath: assessment,
	})
	if err != nil {
		t.Fatal(err)
	}
	if latest.Metrics.ADRTouched == nil || *latest.Metrics.ADRTouched != 2 {
		t.Fatalf("kpi.adr.touched=%v", latest.Metrics.ADRTouched)
	}
	if latest.Metrics.ADRFlagRate == nil || *latest.Metrics.ADRFlagRate != 0.5 {
		t.Fatalf("kpi.adr.flag_rate=%v", latest.Metrics.ADRFlagRate)
	}
	if latest.Links["adr_flagging"] != "adr_flagging.json" {
		t.Fatalf("links=%v", latest.Links)
	}
	var report ADRFlaggingReport
	if err := readJSON(filepath.Join(out, "adr_flagging.json"), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ADRFlaggingSchemaVersion || len(report.ADRs) != 2 {
		t.Fatalf("report=%+v", report)
	}
}
