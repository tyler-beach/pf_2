package calibration

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed params-v1.json
var embeddedParamsJSON []byte

//go:embed analyzer-precision-v1.json
var embeddedAnalyzerPrecisionJSON []byte

const (
	MethodHeuristic           = "heuristic"
	MethodPlattConformal      = "platt_conformal"
	ParamsSchemaVersion       = "confidence-calibration-v1"
	DefaultTargetCoverage     = 0.90
	MinGoldExamplesForFit     = 20
)

// Params is the versioned, offline-fitted calibration artifact.
type Params struct {
	SchemaVersion           string             `json:"schema_version"`
	Method                  string             `json:"method"`
	Version                 string             `json:"version"`
	GoldSchema              string             `json:"gold_schema,omitempty"`
	GoldSHA256              string             `json:"gold_sha256,omitempty"`
	GoldExampleCount        int                `json:"gold_example_count"`
	FeatureNames            []string           `json:"feature_names"`
	Weights                 []float64          `json:"weights"`
	Bias                    float64            `json:"bias"`
	BandHighMin             float64            `json:"band_high_min"`
	BandMediumMin           float64            `json:"band_medium_min"`
	ConformalTargetCoverage float64            `json:"conformal_target_coverage"`
	ConformalQuantile       float64            `json:"conformal_quantile"`
	Notes                   string             `json:"notes,omitempty"`
	ExchangeabilityNote     string             `json:"exchangeability_note,omitempty"`
	AnalyzerPrecision       map[string]float64 `json:"analyzer_precision,omitempty"`
}

// Result is the scored confidence payload attached to candidates / assessments.
// Heuristic triage is an internal feature only; callers see calibrated P + band.
type Result struct {
	ConfidenceP        float64  `json:"confidence_p"`
	ConfidenceBand     string   `json:"confidence_band"`
	ConfidenceMethod   string   `json:"confidence_method"`
	ConfidenceCoverage float64  `json:"confidence_coverage,omitempty"`
	PredictionSet      []string `json:"prediction_set,omitempty"`
	EscalationReasons  []string `json:"escalation_reasons,omitempty"`
	FallbackReason     string   `json:"fallback_reason,omitempty"`
}

var (
	defaultParamsOnce sync.Once
	defaultParams     Params
	defaultParamsErr  error
)

// DefaultParams returns the embedded fitted params (or heuristic fallback).
func DefaultParams() Params {
	defaultParamsOnce.Do(func() {
		defaultParams, defaultParamsErr = ParseParams(embeddedParamsJSON)
		if defaultParamsErr != nil {
			defaultParams = HeuristicParams()
		}
		if defaultParams.AnalyzerPrecision == nil {
			if ap, err := ParseAnalyzerPrecision(embeddedAnalyzerPrecisionJSON); err == nil {
				defaultParams.AnalyzerPrecision = ap.PrecisionByRuleID
			}
		}
	})
	return defaultParams
}

// HeuristicParams is the honest fallback when gold is insufficient or params missing.
func HeuristicParams() Params {
	return Params{
		SchemaVersion:           ParamsSchemaVersion,
		Method:                  MethodHeuristic,
		Version:                 "heuristic-fallback",
		FeatureNames:            append([]string(nil), FeatureNames...),
		BandHighMin:             0.75,
		BandMediumMin:           0.45,
		ConformalTargetCoverage: DefaultTargetCoverage,
		ConformalQuantile:       1,
		Notes:                   "Heuristic triage only; no calibrated P(correct). Label confidence_method=heuristic.",
		ExchangeabilityNote:     "N/A — conformal not active under heuristic fallback.",
	}
}

// ParseParams unmarshals calibration params JSON.
func ParseParams(raw []byte) (Params, error) {
	var p Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return Params{}, err
	}
	if p.SchemaVersion == "" {
		return Params{}, fmt.Errorf("params schema_version required")
	}
	if p.Method == "" {
		p.Method = MethodHeuristic
	}
	if p.ConformalTargetCoverage <= 0 {
		p.ConformalTargetCoverage = DefaultTargetCoverage
	}
	if len(p.FeatureNames) == 0 {
		p.FeatureNames = append([]string(nil), FeatureNames...)
	}
	return p, nil
}

// SufficientGold reports whether a gold set is large enough to claim calibration.
func SufficientGold(n int) bool {
	return n >= MinGoldExamplesForFit
}
