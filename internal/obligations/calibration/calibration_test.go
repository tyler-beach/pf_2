package calibration_test

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

func TestPlattFitApplyMonotonic(t *testing.T) {
	// Separable toy data: high score → correct.
	xs := [][]float64{
		{0.1, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 1, 0},
		{0.2, 0, 1, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{0.8, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0.9, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0.85, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0.15, 0, 0, 1, 1, 1, 0, 0, 0, 0, 1, 1, 0},
	}
	ys := []float64{0, 0, 1, 1, 1, 0}
	w, b := calibration.FitLogistic(xs, ys, 1e-2, 40)
	pLow := calibration.LogisticPredict(w, b, xs[0])
	pHigh := calibration.LogisticPredict(w, b, xs[3])
	if !(pHigh > pLow) {
		t.Fatalf("expected calibrated p high>low, got %v vs %v", pHigh, pLow)
	}
}

func TestConformalCoverageSynthetic(t *testing.T) {
	// Synthetic: true band tracks p thresholds; inductive conformal should
	// cover ≈ target fraction of held-out labels when exchangeable.
	highMin, mediumMin := 0.7, 0.4
	var nonconf []float64
	ps := []float64{0.95, 0.9, 0.85, 0.75, 0.55, 0.5, 0.45, 0.2, 0.1, 0.05, 0.8, 0.6, 0.3, 0.92, 0.15}
	bands := []string{"high", "high", "high", "high", "medium", "medium", "medium", "low", "low", "low", "high", "medium", "low", "high", "low"}
	for i, p := range ps {
		scores := calibration.BandScores(p, highMin, mediumMin)
		nonconf = append(nonconf, 1-scores[bands[i]])
	}
	q := calibration.ConformalQuantile(nonconf, 0.9)
	covered := 0
	for i, p := range ps {
		scores := calibration.BandScores(p, highMin, mediumMin)
		set := calibration.PredictionSet(scores, q)
		for _, s := range set {
			if s == bands[i] {
				covered++
				break
			}
		}
	}
	rate := float64(covered) / float64(len(ps))
	if rate < 0.8 {
		t.Fatalf("synthetic conformal coverage %.2f below 0.8 (q=%v)", rate, q)
	}
	// Singleton high when p is extreme and set is tight.
	scores := calibration.BandScores(0.99, highMin, mediumMin)
	set := calibration.PredictionSet(scores, math.Min(q, 0.3))
	if calibration.BandFromSet(set) == "" {
		t.Fatal("empty band")
	}
}

func TestWilsonInterval(t *testing.T) {
	iv := calibration.WilsonInterval(8, 10, 0.95)
	if iv.Estimate != 0.8 {
		t.Fatalf("estimate %v", iv.Estimate)
	}
	if !(iv.Low < iv.Estimate && iv.Estimate < iv.High) {
		t.Fatalf("interval not ordered: %+v", iv)
	}
	if iv.Low < 0 || iv.High > 1 {
		t.Fatalf("out of bounds: %+v", iv)
	}
	empty := calibration.WilsonInterval(0, 0, 0.95)
	if empty.High != 1 {
		t.Fatalf("empty n should be wide: %+v", empty)
	}
}

func TestApplyHeuristicFallback(t *testing.T) {
	res := calibration.Apply(calibration.FeatureInput{
		HeuristicScore: 0.85,
		Modality:       "must",
		PatternHint:    "configuration.bounds_or_required_values",
	}, "high", nil, calibration.HeuristicParams())
	if res.ConfidenceMethod != calibration.MethodHeuristic {
		t.Fatalf("method %s", res.ConfidenceMethod)
	}
	if res.ConfidenceBand != "high" {
		t.Fatalf("band %s", res.ConfidenceBand)
	}
}

func TestFitGoldAndDefaultParams(t *testing.T) {
	root := repoRoot(t)
	gold, err := calibration.LoadGold(filepath.Join(root, "testdata/confidence/gold-extract-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	ap := calibration.DefaultAnalyzerTable()
	p, err := calibration.FitFromGold(gold, ap.PrecisionByRuleID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != calibration.MethodPlattConformal {
		t.Fatalf("expected platt_conformal, got %s", p.Method)
	}
	if len(p.Weights) != len(calibration.FeatureNames) {
		t.Fatalf("weights len %d", len(p.Weights))
	}
	def := calibration.DefaultParams()
	if def.Method != calibration.MethodPlattConformal {
		t.Fatalf("embedded default method %s", def.Method)
	}
	// Apply a clean high-ish candidate.
	res := calibration.Apply(calibration.FeatureInput{
		HeuristicScore: 0.85,
		Modality:       "must",
		PatternHint:    "configuration.bounds_or_required_values",
	}, "high", nil, def)
	if res.ConfidenceP <= 0 || res.ConfidenceP > 1 {
		t.Fatalf("p out of range %v", res.ConfidenceP)
	}
	if res.ConfidenceMethod != calibration.MethodPlattConformal {
		t.Fatalf("method %s", res.ConfidenceMethod)
	}
	if res.ConfidenceCoverage != 0.9 {
		t.Fatalf("coverage %v", res.ConfidenceCoverage)
	}
	w := calibration.EvidenceWeight("CKV_AWS_24", ap)
	if w < 0.9 {
		t.Fatalf("CKV_AWS_24 weight %v", w)
	}
	if calibration.EvidenceWeight("unknown-rule", ap) != ap.DefaultPrecision {
		t.Fatalf("default weight mismatch")
	}
}

func TestInsufficientGoldFallsBack(t *testing.T) {
	p, err := calibration.FitFromGold(calibration.GoldSet{
		SchemaVersion: "x",
		Examples:      []calibration.GoldExample{{ID: "a"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Method != calibration.MethodHeuristic {
		t.Fatalf("want heuristic, got %s", p.Method)
	}
}
