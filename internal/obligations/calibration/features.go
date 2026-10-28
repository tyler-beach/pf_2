package calibration

import "strings"

// FeatureNames is the ordered feature layout used by Fit and Apply.
// HeuristicScore is an *input* to calibration, never the final probability.
var FeatureNames = []string{
	"heuristic_score",
	"modality_must",
	"modality_should",
	"modality_permissive",
	"manual_review",
	"has_ambiguity",
	"has_exceptions",
	"exception_kind",
	"is_compound",
	"has_conditions",
	"non_decision",
	"subjective",
	"conflicting_modality",
}

// FeatureInput is the extract-side signal bundle used to build a feature vector.
// It intentionally avoids importing obligations to keep this package leaf-level.
type FeatureInput struct {
	HeuristicScore float64
	Modality       string
	PatternHint    string
	Kind           string
	Ambiguity      []string
	Exceptions     []string
	Conditions     []string
	ReviewHints    []string
}

// Vector returns the ordered feature values matching FeatureNames.
func Vector(in FeatureInput) []float64 {
	v := make([]float64, len(FeatureNames))
	v[0] = clamp01(in.HeuristicScore)
	switch strings.ToLower(strings.TrimSpace(in.Modality)) {
	case "must", "must_not":
		v[1] = 1
	case "should":
		v[2] = 1
	case "may", "not_required":
		v[3] = 1
	default:
		v[3] = 1
	}
	if in.PatternHint == "" || in.PatternHint == "manual_review" {
		v[4] = 1
	}
	if len(in.Ambiguity) > 0 {
		v[5] = 1
	}
	if len(in.Exceptions) > 0 {
		v[6] = 1
	}
	if in.Kind == "exception_candidate" {
		v[7] = 1
	}
	if hasAny(in.ReviewHints, "compound_or_conjoined", "split_from_compound") {
		v[8] = 1
	}
	if len(in.Conditions) > 0 || hasAny(in.ReviewHints, "conditional_scope") {
		v[9] = 1
	}
	if hasAny(in.ReviewHints, "non_decision_section") {
		v[10] = 1
	}
	if hasAny(in.ReviewHints, "subjective_or_underspecified", "ambiguous_interpretation") {
		v[11] = 1
	}
	if hasAny(in.ReviewHints, "conflicting_modality_cues") {
		v[12] = 1
	}
	return v
}

// VectorFromGoldFeatures builds a vector from gold JSON feature objects.
func VectorFromGoldFeatures(f GoldFeatures) []float64 {
	hints := make([]string, 0, 6)
	if f.IsCompound {
		hints = append(hints, "compound_or_conjoined")
	}
	if f.NonDecisionSection {
		hints = append(hints, "non_decision_section")
	}
	if f.Subjective {
		hints = append(hints, "subjective_or_underspecified")
	}
	if f.ConflictingModality {
		hints = append(hints, "conflicting_modality_cues")
	}
	var amb, exc, cond []string
	if f.HasAmbiguity {
		amb = []string{"flag"}
	}
	if f.HasExceptions {
		exc = []string{"flag"}
	}
	if f.HasConditions {
		cond = []string{"flag"}
	}
	kind := f.Kind
	if kind == "" && f.IsExceptionKind {
		kind = "exception_candidate"
	}
	hint := f.PatternHint
	if f.IsManualReview {
		hint = "manual_review"
	}
	return Vector(FeatureInput{
		HeuristicScore: f.HeuristicScore,
		Modality:       f.Modality,
		PatternHint:    hint,
		Kind:           kind,
		Ambiguity:      amb,
		Exceptions:     exc,
		Conditions:     cond,
		ReviewHints:    hints,
	})
}

func hasAny(hints []string, want ...string) bool {
	set := map[string]bool{}
	for _, h := range hints {
		set[h] = true
	}
	for _, w := range want {
		if set[w] {
			return true
		}
	}
	return false
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
