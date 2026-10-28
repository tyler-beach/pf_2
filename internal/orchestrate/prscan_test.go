package orchestrate

import (
	"context"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/prscan"
)

func TestFindingConverterRejectsLLMTaintedAssessment(t *testing.T) {
	conv := NewDefaultFindingConverter()
	_, err := conv.Convert(context.Background(), assessment.Result{LLMCalls: 2, Verdict: "PASS"}, prscan.ConvertOptions{})
	if err == nil || !strings.Contains(err.Error(), "runtime LLM") {
		t.Fatalf("expected LLM taint rejection, got %v", err)
	}
}

func TestFindingConverterPassesDeterministic(t *testing.T) {
	conv := NewDefaultFindingConverter()
	out, err := conv.Convert(context.Background(), assessment.Result{
		LLMCalls: 0, Verdict: "PASS", RequirementCount: 0,
	}, prscan.ConvertOptions{MaxInline: 5})
	if err != nil {
		t.Fatal(err)
	}
	if out.Summary.Posted != 0 {
		t.Fatalf("expected empty findings for empty assessment, got %+v", out.Summary)
	}
}
