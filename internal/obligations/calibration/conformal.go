package calibration

import (
	"math"
	"sort"
)

const (
	BandHigh   = "high"
	BandMedium = "medium"
	BandLow    = "low"
)

// BandScores maps a calibrated P(correct) into soft class scores for
// high / medium / low using ordered thresholds on p.
// The point band from thresholds gets the majority mass; neighbors share the remainder
// so conformal sets can widen without immediately becoming the full {high,medium,low}.
func BandScores(p, highMin, mediumMin float64) map[string]float64 {
	point := bandFromScore(p, highMin, mediumMin)
	// Sharpness from distance to decision boundaries.
	sharp := 0.55
	switch point {
	case BandHigh:
		sharp = 0.55 + 0.4*clamp01((p-highMin)/math.Max(1e-6, 1-highMin))
	case BandLow:
		sharp = 0.55 + 0.4*clamp01((mediumMin-p)/math.Max(1e-6, mediumMin))
	default:
		mid := (highMin + mediumMin) / 2
		span := math.Max(1e-6, (highMin-mediumMin)/2)
		sharp = 0.55 + 0.35*(1-math.Min(1, math.Abs(p-mid)/span))
	}
	rest := (1 - sharp) / 2
	scores := map[string]float64{BandHigh: rest, BandMedium: rest, BandLow: rest}
	scores[point] = sharp
	return scores
}

// ConformalQuantile returns the inductive conformal quantile for target coverage.
// Exchangeability assumption: calibration examples and future candidates are
// exchangeable draws from the same extract process (same corpus/distribution).
func ConformalQuantile(nonconformities []float64, targetCoverage float64) float64 {
	if len(nonconformities) == 0 {
		return 1
	}
	if targetCoverage <= 0 {
		targetCoverage = 0.9
	}
	if targetCoverage >= 1 {
		return 1
	}
	sorted := append([]float64(nil), nonconformities...)
	sort.Float64s(sorted)
	n := len(sorted)
	// Finite-sample correction: ceil((n+1)*coverage)/n index (1-based → 0-based).
	idx := int(math.Ceil((float64(n)+1)*targetCoverage)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		return 1
	}
	return sorted[idx]
}

// PredictionSet returns labels whose nonconformity (1 - score) ≤ q.
func PredictionSet(scores map[string]float64, q float64) []string {
	order := []string{BandHigh, BandMedium, BandLow}
	var set []string
	for _, b := range order {
		if 1-scores[b] <= q+1e-12 {
			set = append(set, b)
		}
	}
	if len(set) == 0 {
		// Fail closed to widest band.
		return []string{BandLow}
	}
	return set
}

// BandFromSet maps a conformal prediction set to a triage band:
// singleton {high} → high; otherwise widen (medium if only high/medium, else low).
func BandFromSet(set []string) string {
	has := map[string]bool{}
	for _, s := range set {
		has[s] = true
	}
	if len(set) == 1 && has[BandHigh] {
		return BandHigh
	}
	if has[BandLow] || (has[BandHigh] && has[BandMedium] && has[BandLow]) {
		return BandLow
	}
	if has[BandMedium] || (has[BandHigh] && has[BandMedium]) {
		return BandMedium
	}
	if has[BandHigh] {
		return BandHigh
	}
	return BandLow
}

// FitBandThresholds picks high/medium floors on calibrated p from gold expected bands.
func FitBandThresholds(ps []float64, bands []string) (highMin, mediumMin float64) {
	highMin, mediumMin = 0.75, 0.45
	if len(ps) == 0 || len(ps) != len(bands) {
		return highMin, mediumMin
	}
	var highPs, medPs, lowPs []float64
	for i, b := range bands {
		switch b {
		case BandHigh:
			highPs = append(highPs, ps[i])
		case BandMedium:
			medPs = append(medPs, ps[i])
		default:
			lowPs = append(lowPs, ps[i])
		}
	}
	if len(highPs) > 0 && len(medPs) > 0 {
		highMin = (percentile(highPs, 0.2) + percentile(medPs, 0.8)) / 2
	} else if len(highPs) > 0 {
		highMin = percentile(highPs, 0.2)
	}
	if len(medPs) > 0 && len(lowPs) > 0 {
		mediumMin = (percentile(medPs, 0.2) + percentile(lowPs, 0.8)) / 2
	} else if len(medPs) > 0 {
		mediumMin = percentile(medPs, 0.2)
	}
	if mediumMin > highMin {
		mediumMin = highMin * 0.7
	}
	if mediumMin < 0.05 {
		mediumMin = 0.05
	}
	if highMin > 0.95 {
		highMin = 0.95
	}
	if highMin <= mediumMin {
		highMin = mediumMin + 0.1
	}
	return round4(highMin), round4(mediumMin)
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if p <= 0 {
		return s[0]
	}
	if p >= 1 {
		return s[len(s)-1]
	}
	idx := p * float64(len(s)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return s[lo]
	}
	w := idx - float64(lo)
	return s[lo]*(1-w) + s[hi]*w
}

func round4(x float64) float64 {
	return math.Round(x*10000) / 10000
}
