package workshop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func valueOrNaN(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func TestComputeAssessOutcomeMix(t *testing.T) {
	mix := ComputeAssessOutcomeMix(map[string]int{
		"NOT_APPLICABLE": 80,
		"PASS":           10,
		"FAIL":           5,
		"INDETERMINATE":  4,
		"WARN":           1,
	}, 100, 20)
	if mix.ApplicableCount != 20 || mix.NACount != 80 {
		t.Fatalf("counts: %+v", mix)
	}
	if mix.NARate == nil || *mix.NARate != 0.8 {
		t.Fatalf("na_rate: %v", mix.NARate)
	}
	if mix.PassShare == nil || *mix.PassShare != 0.5 {
		t.Fatalf("pass_share: %v", mix.PassShare)
	}
	if mix.FailShare == nil || *mix.FailShare != 0.25 {
		t.Fatalf("fail_share: %v", mix.FailShare)
	}
	if mix.WarnShare == nil || *mix.WarnShare != 0.05 {
		t.Fatalf("warn_share: %v", mix.WarnShare)
	}
}

func TestComputeCorpusFunnelRates(t *testing.T) {
	f := ComputeCorpusFunnelRates(200, 200, 180, 100, 50)
	if f.AcceptedRate == nil || *f.AcceptedRate != 1 {
		t.Fatalf("accepted rate: %v", f.AcceptedRate)
	}
	if f.BoundRate == nil || *f.BoundRate != 0.5 {
		t.Fatalf("bound rate: %v", f.BoundRate)
	}
	if f.EvidenceCompleteRate == nil || *f.EvidenceCompleteRate != 0.25 {
		t.Fatalf("evidence rate: %v", f.EvidenceCompleteRate)
	}
}

func TestComputeKindQualityAndEmpiricalCoverage(t *testing.T) {
	cands := []kindCand{
		{SubmissionKind: "tech_spec", PatternHint: "manual_review", ConfidenceP: 0.4, Confidence: "low", PredictionSet: []string{"low", "medium"}},
		{SubmissionKind: "tech_spec", PatternHint: "security.source_pattern", ConfidenceP: 0.8, Confidence: "high", PredictionSet: []string{"high"}},
		{SubmissionKind: "code_snippet", PatternHint: "security.source_pattern", ConfidenceP: 0.9, Confidence: "high", PredictionSet: []string{"medium"}},
		{SubmissionKind: "", PatternHint: "x", ConfidenceP: 0.1, Confidence: "low"},
	}
	kq := ComputeKindQualityStats(cands, 4)
	if kq.TechSpecMeanP == nil || *kq.TechSpecMeanP < 0.599 || *kq.TechSpecMeanP > 0.601 {
		t.Fatalf("tech mean_p: %v", valueOrNaN(kq.TechSpecMeanP))
	}
	if kq.TechSpecGapRate == nil || *kq.TechSpecGapRate != 0.5 {
		t.Fatalf("tech gap: %v", kq.TechSpecGapRate)
	}
	if kq.UnspecifiedRate == nil || *kq.UnspecifiedRate != 0.25 {
		t.Fatalf("unspecified: %v", kq.UnspecifiedRate)
	}
	emp := EmpiricalPredictionSetCoverage(cands)
	// 2 of 3 with prediction sets contain assigned band
	if emp == nil || *emp < 0.66 || *emp > 0.67 {
		t.Fatalf("empirical coverage: %v", emp)
	}
}

func TestBuildRuleHits(t *testing.T) {
	report := BuildRuleHits("run-x", []ADRFlaggingInput{{
		RunID: "run-x", SubmissionKind: "code_snippet",
		Requirements: []ADRFlaggingInputRequirement{
			{ADRID: "ADR-1", RequirementID: "R1", Status: "FAIL", Findings: []ADRFlaggingRule{
				{RuleID: "pathfinder.go-shell", Analyzer: "opengrep"},
			}},
			{ADRID: "ADR-2", RequirementID: "R2", Status: "PASS", Findings: []ADRFlaggingRule{
				{RuleID: "pathfinder.go-shell", Analyzer: "opengrep"},
			}},
			{ADRID: "ADR-3", RequirementID: "R3", Status: "FAIL", Findings: []ADRFlaggingRule{
				{RuleID: "CKV_AWS_1", Analyzer: "checkov"},
			}},
		},
	}})
	if report.Summary.DistinctRules != 2 || report.Summary.HitTotal != 3 {
		t.Fatalf("summary: %+v", report.Summary)
	}
	if report.Summary.TopRuleID != "pathfinder.go-shell" || report.Summary.TopHitCount != 2 {
		t.Fatalf("top: %+v", report.Summary)
	}
	if report.Summary.FlagTotal != 2 {
		t.Fatalf("flag total: %d", report.Summary.FlagTotal)
	}
}

func TestPublishFreshnessSLO(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	age, ok := PublishFreshnessSLO(now.Add(-30*time.Minute).Format(time.RFC3339), now)
	if !ok || age != 30 {
		t.Fatalf("fresh: age=%d ok=%v", age, ok)
	}
	age, ok = PublishFreshnessSLO(now.Add(-5*time.Hour).Format(time.RFC3339), now)
	if ok || age != 300 {
		t.Fatalf("stale: age=%d ok=%v", age, ok)
	}
}

func TestCostPerUsefulOutcome(t *testing.T) {
	perAcc, perFail := CostPerUsefulOutcome(2.0, 4, 2)
	if perAcc == nil || *perAcc != 0.5 {
		t.Fatalf("per accepted: %v", perAcc)
	}
	if perFail == nil || *perFail != 1.0 {
		t.Fatalf("per fail: %v", perFail)
	}
}

func TestEmitKPIPackSurfacesNewFields(t *testing.T) {
	root := t.TempDir()
	candidates := filepath.Join(root, "candidates.json")
	assessment := filepath.Join(root, "assessment.json")
	harness := filepath.Join(root, "harness.json")
	if err := os.WriteFile(candidates, []byte(`{
		"candidate_count": 3,
		"confidence_summary": {"high": 1, "medium": 1, "low": 1},
		"calibration": {"method": "platt_conformal", "coverage": 0.9, "gold_example_count": 81, "params_version": "v1"},
		"candidates": [
			{"pattern_hint": "manual_review", "submission_kind": "tech_spec", "confidence": "low", "confidence_p": 0.4, "confidence_method": "platt_conformal", "prediction_set": ["low"]},
			{"pattern_hint": "security.source_pattern", "submission_kind": "tech_spec", "confidence": "medium", "confidence_p": 0.7, "confidence_method": "platt_conformal", "prediction_set": ["medium"]},
			{"pattern_hint": "security.source_pattern", "submission_kind": "code_snippet", "confidence": "high", "confidence_p": 0.9, "confidence_method": "platt_conformal", "prediction_set": ["high"]}
		]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assessment, []byte(`{
		"verdict": "FAIL",
		"runtime_llm_calls": 0,
		"submission_kind": "mixed",
		"requirement_count": 10,
		"applicable_requirement_count": 4,
		"counts": {"NOT_APPLICABLE": 6, "PASS": 1, "FAIL": 2, "INDETERMINATE": 1},
		"corpus_coverage": {
			"inventory_adrs": 10,
			"accepted_adrs": 10,
			"interpreted_requirements": 8,
			"bound_requirements": 4,
			"evidence_complete": 2
		},
		"requirements": [
			{"adr_id": "ADR-1", "requirement_id": "R1", "status": "FAIL", "title": "t",
				"findings": [{"rule_id": "pathfinder.go-shell", "analyzer": "opengrep"}]}
		]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harness, []byte(`{
		"precision": 0.8,
		"recall": 0.75,
		"matched_candidates": 8,
		"false_positives": 2,
		"precision_by_confidence": {
			"high": {"precision": 0.9},
			"medium": {"precision": 0.8},
			"low": {"precision": 0.5}
		},
		"recall_by_confidence": {
			"high": {"recall": 0.85},
			"medium": {"recall": 0.7},
			"low": {"recall": 0.6}
		},
		"by_submission_kind": {
			"tech_spec": {"precision": 0.82, "recall": 0.7},
			"code_snippet": {"precision": 0.78, "recall": 0.8}
		}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "snapshot")
	ts := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	latest, err := Emit(EmitOptions{
		OutDir:         out,
		Run:            RunIdentity{RunID: "kpi-pack", Workflow: "CI", Timestamp: ts},
		CandidatesPath: candidates,
		AssessmentPath: assessment,
		HarnessPath:    harness,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := latest.Metrics
	if m.AssessSubmissionKind != "mixed" {
		t.Fatalf("kind: %q", m.AssessSubmissionKind)
	}
	if m.AssessNARate == nil || *m.AssessNARate != 0.6 {
		t.Fatalf("na_rate: %v", m.AssessNARate)
	}
	if m.AssessApplicableCount == nil || *m.AssessApplicableCount != 4 {
		t.Fatalf("applicable: %v", m.AssessApplicableCount)
	}
	if m.AssessFunnelBoundRate == nil || *m.AssessFunnelBoundRate != 0.4 {
		t.Fatalf("funnel bound: %v", m.AssessFunnelBoundRate)
	}
	if m.ConfidenceParamsVersion != "v1" || m.ConfidenceGoldSize == nil || *m.ConfidenceGoldSize != 81 {
		t.Fatalf("calibration meta: ver=%s gold=%v", m.ConfidenceParamsVersion, m.ConfidenceGoldSize)
	}
	if m.ConfidenceEmpiricalCoverage == nil || *m.ConfidenceEmpiricalCoverage != 1 {
		t.Fatalf("empirical: %v", m.ConfidenceEmpiricalCoverage)
	}
	if m.HarnessFalsePositives == nil || *m.HarnessFalsePositives != 2 {
		t.Fatalf("fp: %v", m.HarnessFalsePositives)
	}
	if m.HarnessFPRate == nil || *m.HarnessFPRate != 0.2 {
		t.Fatalf("fp_rate: %v", m.HarnessFPRate)
	}
	if m.HarnessPrecisionTechSpec == nil || *m.HarnessPrecisionTechSpec != 0.82 {
		t.Fatalf("harness tech: %v", m.HarnessPrecisionTechSpec)
	}
	if m.HarnessPrecisionHigh == nil || *m.HarnessPrecisionHigh != 0.9 {
		t.Fatalf("precision high: %v", m.HarnessPrecisionHigh)
	}
	if m.ConfidenceShareMediumLo == nil {
		t.Fatal("expected medium share CI")
	}
	if m.RuleDistinctCount == nil || *m.RuleDistinctCount != 1 {
		t.Fatalf("rules: %v", m.RuleDistinctCount)
	}
	if latest.SLO["publish_freshness_ok"] != true {
		t.Fatalf("freshness SLO: %+v", latest.SLO)
	}
	if _, err := os.Stat(filepath.Join(out, "rule_hits.json")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "metrics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if round["kpi.assess.na_rate"] == nil {
		t.Fatalf("metrics missing na_rate: %s", raw)
	}
}
