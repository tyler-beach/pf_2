package calibration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// FitFromGold fits Platt logistic + inductive conformal band params from gold.
// When gold is below MinGoldExamplesForFit, returns HeuristicParams labeled honestly.
func FitFromGold(gold GoldSet, analyzerPrecision map[string]float64) (Params, error) {
	if !SufficientGold(len(gold.Examples)) {
		p := HeuristicParams()
		p.GoldSchema = gold.SchemaVersion
		p.GoldExampleCount = len(gold.Examples)
		p.Notes = fmt.Sprintf("Gold insufficient (%d < %d); heuristic fallback.", len(gold.Examples), MinGoldExamplesForFit)
		p.AnalyzerPrecision = analyzerPrecision
		return p, nil
	}

	xs := make([][]float64, 0, len(gold.Examples))
	ys := make([]float64, 0, len(gold.Examples))
	bands := make([]string, 0, len(gold.Examples))
	for _, ex := range gold.Examples {
		xs = append(xs, VectorFromGoldFeatures(ex.Features))
		if ex.Label.ExtractCorrect {
			ys = append(ys, 1)
		} else {
			ys = append(ys, 0)
		}
		b := ex.Label.ExpectedBand
		if b == "" {
			b = ex.Features.HeuristicBand
		}
		if b == "" {
			b = BandLow
		}
		bands = append(bands, b)
	}

	weights, bias := FitLogistic(xs, ys, 1e-2, 50)
	ps := make([]float64, len(xs))
	for i, x := range xs {
		ps[i] = LogisticPredict(weights, bias, x)
	}
	highMin, mediumMin := FitBandThresholds(ps, bands)

	nonconf := make([]float64, 0, len(xs))
	for i, p := range ps {
		scores := BandScores(p, highMin, mediumMin)
		trueBand := bands[i]
		s := scores[trueBand]
		nonconf = append(nonconf, 1-s)
	}
	q := ConformalQuantile(nonconf, DefaultTargetCoverage)

	raw, _ := json.Marshal(gold)
	sum := sha256.Sum256(raw)

	return Params{
		SchemaVersion:           ParamsSchemaVersion,
		Method:                  MethodPlattConformal,
		Version:                 "v1",
		GoldSchema:              gold.SchemaVersion,
		GoldSHA256:              hex.EncodeToString(sum[:]),
		GoldExampleCount:        len(gold.Examples),
		FeatureNames:            append([]string(nil), FeatureNames...),
		Weights:                 weights,
		Bias:                    bias,
		BandHighMin:             highMin,
		BandMediumMin:           mediumMin,
		ConformalTargetCoverage: DefaultTargetCoverage,
		ConformalQuantile:       round4(q),
		Notes:                   "Platt/logistic on extract feature vector (heuristic_score is one feature). Conformal bands use inductive quantile at target coverage under exchangeability.",
		ExchangeabilityNote:     "Assumes future candidates are exchangeable with gold calibration examples (same extract process / corpus family). Distribution shift (new domains, parsers, or the 50-submissions pack) requires refit.",
		AnalyzerPrecision:       analyzerPrecision,
	}, nil
}

// WriteParamsJSON writes params with stable formatting.
func WriteParamsJSON(path string, p Params) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}
