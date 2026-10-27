package workshop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmitAggregatesKPIsAndFailsClosedOnAssessLLM(t *testing.T) {
	root := t.TempDir()
	candidates := filepath.Join(root, "candidates.json")
	assessment := filepath.Join(root, "assessment.json")
	if err := os.WriteFile(candidates, []byte(`{
		"candidate_count": 4,
		"confidence_summary": {"high": 2, "medium": 1, "low": 1},
		"candidates": [
			{"pattern_hint": "manual_review"},
			{"pattern_hint": "configuration.bounds_or_required_values"},
			{"pattern_hint": "configuration.bounds_or_required_values"},
			{"pattern_hint": "security.source_pattern"}
		]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assessment, []byte(`{"verdict":"PASS","runtime_llm_calls":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "snapshot")
	latest, err := Emit(EmitOptions{
		OutDir:         out,
		Run:            RunIdentity{RunID: "run-1", Workflow: "CI", GitSHA: "abc"},
		CandidatesPath: candidates,
		AssessmentPath: assessment,
	})
	if err != nil {
		t.Fatal(err)
	}
	if latest.Metrics.CandidateTotal != 4 || latest.Metrics.ConfidenceLow != 1 {
		t.Fatalf("unexpected metrics: %+v", latest.Metrics)
	}
	if latest.Metrics.EscalationRate != 0.25 {
		t.Fatalf("escalation rate: %v", latest.Metrics.EscalationRate)
	}
	if latest.SLO["assess_runtime_llm_calls_zero"] != true {
		t.Fatalf("SLO missing: %+v", latest.SLO)
	}
	raw, err := os.ReadFile(filepath.Join(out, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip Latest
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(root, "bad-assessment.json")
	if err := os.WriteFile(bad, []byte(`{"verdict":"PASS","runtime_llm_calls":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Emit(EmitOptions{
		OutDir:           filepath.Join(root, "bad"),
		Run:              RunIdentity{RunID: "run-2", Workflow: "ADR Assessment"},
		AssessmentPath:   bad,
		EnforceAssessLLM: true,
	})
	if err == nil {
		t.Fatal("expected fail-closed on runtime LLM calls")
	}
}
