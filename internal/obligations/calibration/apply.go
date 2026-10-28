package calibration

import "math"

// Apply scores a feature input with params. Unspecified / broken params fall back
// to heuristic banding and label confidence_method=heuristic.
func Apply(in FeatureInput, heuristicBand string, heuristicReasons []string, params Params) Result {
	score := clamp01(in.HeuristicScore)
	score = math.Round(score*100) / 100

	if params.Method == "" || params.Method == MethodHeuristic || len(params.Weights) == 0 {
		band := heuristicBand
		if band == "" {
			band = bandFromScore(score, params.BandHighMin, params.BandMediumMin)
		}
		return Result{
			HeuristicScore:     score,
			ConfidenceP:        score, // honestly not calibrated; same numeric for back-compat consumers
			ConfidenceBand:     band,
			ConfidenceMethod:   MethodHeuristic,
			ConfidenceCoverage: 0,
			EscalationReasons:  append([]string(nil), heuristicReasons...),
			FallbackReason:     "calibration params missing or method=heuristic",
		}
	}

	x := Vector(in)
	if len(params.Weights) != len(x) {
		band := heuristicBand
		if band == "" {
			band = bandFromScore(score, 0.75, 0.45)
		}
		return Result{
			HeuristicScore:    score,
			ConfidenceP:       score,
			ConfidenceBand:    band,
			ConfidenceMethod:  MethodHeuristic,
			EscalationReasons: append([]string(nil), heuristicReasons...),
			FallbackReason:    "feature dimension mismatch; heuristic fallback",
		}
	}

	p := LogisticPredict(params.Weights, params.Bias, x)
	p = math.Round(p*10000) / 10000
	highMin := params.BandHighMin
	mediumMin := params.BandMediumMin
	if highMin <= 0 {
		highMin = 0.75
	}
	if mediumMin <= 0 {
		mediumMin = 0.45
	}
	scores := BandScores(p, highMin, mediumMin)
	q := params.ConformalQuantile
	if q <= 0 {
		q = 1
	}
	set := PredictionSet(scores, q)
	band := BandFromSet(set)
	point := bandFromScore(p, highMin, mediumMin)
	// If the conformal set is vacuous (all labels), keep statistical coverage
	// metadata but report the calibrated point band rather than forcing low.
	if len(set) >= 3 {
		band = point
	}
	coverage := params.ConformalTargetCoverage
	if coverage <= 0 {
		coverage = DefaultTargetCoverage
	}

	reasons := append([]string(nil), heuristicReasons...)
	if len(set) > 1 && len(set) < 3 && band != BandHigh {
		reasons = appendUnique(reasons, "conformal_set_widened")
	}
	if len(set) >= 3 {
		reasons = appendUnique(reasons, "conformal_set_vacuous_point_band")
	}
	if p < mediumMin {
		reasons = appendUnique(reasons, "calibrated_p_below_medium")
	}

	return Result{
		HeuristicScore:     score,
		ConfidenceP:        p,
		ConfidenceBand:     band,
		ConfidenceMethod:   MethodPlattConformal,
		ConfidenceCoverage: coverage,
		PredictionSet:      set,
		EscalationReasons:  reasons,
	}
}

func bandFromScore(score, highMin, mediumMin float64) string {
	switch {
	case score >= highMin:
		return BandHigh
	case score >= mediumMin:
		return BandMedium
	default:
		return BandLow
	}
}

func appendUnique(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	return append(xs, s)
}
