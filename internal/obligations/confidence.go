package obligations

import (
	"strings"

	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// ScoreCandidate runs internal heuristic features through gold-fitted calibration
// (Platt + conformal). Emitted fields are confidence_p / band / method — never a
// public confidence_score. When params are missing, method=heuristic is labeled.
func ScoreCandidate(c Candidate) calibration.Result {
	band, score, reasons := scoreConfidence(c)
	return calibration.Apply(calibration.FeatureInput{
		HeuristicScore: score,
		Modality:       c.Modality,
		PatternHint:    c.PatternHint,
		Kind:           c.Kind,
		Ambiguity:      c.Ambiguity,
		Exceptions:     c.Exceptions,
		Conditions:     c.Conditions,
		ReviewHints:    c.ReviewHints,
	}, band, reasons, calibration.DefaultParams())
}

// scoreConfidence assigns an internal heuristic triage score/band used only as a
// calibration feature. It never consults an LLM and is not emitted on candidates.
func scoreConfidence(c Candidate) (band string, score float64, reasons []string) {
	reasons = make([]string, 0, 4)
	score = 0.85

	switch c.Modality {
	case "must", "must_not":
		// keep base
	case "should":
		score -= 0.15
		reasons = append(reasons, "should_modality")
	case "may", "not_required":
		score -= 0.25
		reasons = append(reasons, "permissive_or_negative_requirement_modality")
	default:
		score -= 0.35
		reasons = append(reasons, "unknown_modality")
	}

	if c.PatternHint == "" || c.PatternHint == "manual_review" {
		score -= 0.25
		reasons = append(reasons, "manual_review_pattern_hint")
	}
	if c.Kind == "exception_candidate" || len(c.Exceptions) > 0 {
		score -= 0.2
		reasons = append(reasons, "exception_language")
	}
	if len(c.Ambiguity) > 0 {
		score -= 0.25
		reasons = append(reasons, "ambiguity_flags")
	}
	if hasHint(c.ReviewHints, "subjective_or_underspecified") || hasHint(c.ReviewHints, "ambiguous_interpretation") {
		score -= 0.15
		reasons = appendUniqueReason(reasons, "subjective_or_underspecified")
	}
	if hasHint(c.ReviewHints, "non_decision_section") {
		score -= 0.2
		reasons = append(reasons, "non_decision_section")
	}
	if hasHint(c.ReviewHints, "compound_or_conjoined") || hasHint(c.ReviewHints, "split_from_compound") {
		score -= 0.1
		reasons = append(reasons, "compound_statement")
	}
	if hasHint(c.ReviewHints, "conditional_scope") || len(c.Conditions) > 0 {
		score -= 0.1
		reasons = append(reasons, "conditional_scope")
	}
	if hasHint(c.ReviewHints, "conflicting_modality_cues") {
		score -= 0.2
		reasons = append(reasons, "conflicting_modality_cues")
	}

	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	score = float64(int(score*100+0.5)) / 100

	switch {
	case score >= 0.75 && len(intersectReasons(reasons, lowForceReasons())) == 0:
		band = ConfidenceHigh
	case score >= 0.45:
		band = ConfidenceMedium
	default:
		band = ConfidenceLow
	}
	// Force low for hard ambiguity / manual review even if score rounded up.
	if c.PatternHint == "manual_review" || hasHint(c.ReviewHints, "subjective_or_underspecified") ||
		hasHint(c.ReviewHints, "ambiguous_interpretation") || hasHint(c.ReviewHints, "conflicting_modality_cues") ||
		hasHint(c.ReviewHints, "non_decision_section") {
		if band == ConfidenceHigh {
			band = ConfidenceMedium
		}
		if c.PatternHint == "manual_review" && (hasHint(c.ReviewHints, "subjective_or_underspecified") ||
			hasHint(c.ReviewHints, "ambiguous_interpretation") || len(c.Ambiguity) > 0) {
			band = ConfidenceLow
		}
	}
	if band == ConfidenceLow && len(reasons) == 0 {
		reasons = append(reasons, "low_score")
	}
	return band, score, reasons
}

func lowForceReasons() []string {
	return []string{"ambiguity_flags", "subjective_or_underspecified", "manual_review_pattern_hint", "non_decision_section", "conflicting_modality_cues"}
}

func intersectReasons(have, want []string) []string {
	set := map[string]bool{}
	for _, r := range have {
		set[r] = true
	}
	out := []string{}
	for _, r := range want {
		if set[r] {
			out = append(out, r)
		}
	}
	return out
}

func hasHint(hints []string, want string) bool {
	for _, h := range hints {
		if h == want {
			return true
		}
	}
	return false
}

func appendUniqueReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

// ConfidenceSummary counts candidates by confidence band.
type ConfidenceSummary struct {
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
}

// SummarizeConfidence returns band counts for an index.
func SummarizeConfidence(index Index) ConfidenceSummary {
	var s ConfidenceSummary
	for _, c := range index.Candidates {
		switch strings.ToLower(strings.TrimSpace(c.Confidence)) {
		case ConfidenceHigh:
			s.High++
		case ConfidenceMedium:
			s.Medium++
		default:
			s.Low++
		}
	}
	return s
}

// FilterByMaxConfidence returns candidates at or below the named band.
// high includes all; medium includes medium+low; low includes only low.
func FilterByMaxConfidence(index Index, maxBand string) []Candidate {
	maxBand = strings.ToLower(strings.TrimSpace(maxBand))
	out := make([]Candidate, 0, len(index.Candidates))
	for _, c := range index.Candidates {
		band := strings.ToLower(strings.TrimSpace(c.Confidence))
		switch maxBand {
		case ConfidenceHigh, "":
			out = append(out, c)
		case ConfidenceMedium:
			if band == ConfidenceMedium || band == ConfidenceLow {
				out = append(out, c)
			}
		case ConfidenceLow:
			if band == ConfidenceLow {
				out = append(out, c)
			}
		default:
			if band == maxBand {
				out = append(out, c)
			}
		}
	}
	return out
}
