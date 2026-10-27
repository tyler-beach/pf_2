package obligations

import "testing"

func TestScoreConfidenceBands(t *testing.T) {
	high := Candidate{
		Modality: "must", PatternHint: "configuration.bounds_or_required_values",
		Kind: "normative_candidate",
	}
	band, score, reasons := scoreConfidence(high)
	if band != ConfidenceHigh || score < 0.75 {
		t.Fatalf("expected high confidence, got band=%s score=%v reasons=%v", band, score, reasons)
	}

	low := Candidate{
		Modality: "should", PatternHint: "manual_review",
		Kind: "normative_candidate",
		ReviewHints: []string{"subjective_or_underspecified", "ambiguous_interpretation"},
		Ambiguity: []string{"subjective_or_preference_language"},
	}
	band, score, reasons = scoreConfidence(low)
	if band != ConfidenceLow {
		t.Fatalf("expected low confidence, got band=%s score=%v reasons=%v", band, score, reasons)
	}
	if len(reasons) == 0 {
		t.Fatal("low confidence must record escalation reasons")
	}

	medium := Candidate{
		Modality: "must", PatternHint: "security.source_pattern",
		Kind: "normative_candidate",
		ReviewHints: []string{"compound_or_conjoined", "split_from_compound"},
	}
	band, _, _ = scoreConfidence(medium)
	if band != ConfidenceHigh && band != ConfidenceMedium {
		t.Fatalf("compound must stay medium/high, got %s", band)
	}
}

func TestFilterByMaxConfidence(t *testing.T) {
	index := Index{Candidates: []Candidate{
		{ID: "a", Confidence: ConfidenceHigh},
		{ID: "b", Confidence: ConfidenceMedium},
		{ID: "c", Confidence: ConfidenceLow},
	}}
	if got := FilterByMaxConfidence(index, ConfidenceLow); len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("low filter = %+v", got)
	}
	if got := FilterByMaxConfidence(index, ConfidenceMedium); len(got) != 2 {
		t.Fatalf("medium filter = %+v", got)
	}
	if got := FilterByMaxConfidence(index, ConfidenceHigh); len(got) != 3 {
		t.Fatalf("high filter = %+v", got)
	}
}
