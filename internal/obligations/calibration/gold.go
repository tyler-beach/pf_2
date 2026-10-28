package calibration

import (
	"encoding/json"
	"fmt"
	"os"
)

// GoldSet is the versioned extract-quality gold schema.
type GoldSet struct {
	SchemaVersion string        `json:"schema_version"`
	Task          string        `json:"task"`
	Description   string        `json:"description,omitempty"`
	LabelSchema   map[string]string `json:"label_schema,omitempty"`
	Examples      []GoldExample `json:"examples"`
}

// GoldExample is one labeled extract instance (or synthetic seed).
type GoldExample struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	ADRID     string         `json:"adr_id,omitempty"`
	Statement string         `json:"statement,omitempty"`
	Label     GoldLabel      `json:"label"`
	Features  GoldFeatures   `json:"features"`
	Hooks     map[string]any `json:"hooks,omitempty"`
}

// GoldLabel holds extract quality + band targets; assess/assist hooks grow later.
type GoldLabel struct {
	ExtractCorrect     bool   `json:"extract_correct"`
	ExpectedBand       string `json:"expected_band"`
	EscalateExpected   bool   `json:"escalate_expected"`
	ExpectedModality   string `json:"expected_modality,omitempty"`
	ExpectedPatternID  string `json:"expected_pattern_id,omitempty"`
}

// GoldFeatures mirrors extract signals used as the calibration feature vector.
type GoldFeatures struct {
	HeuristicScore       float64 `json:"heuristic_score"`
	Modality             string  `json:"modality"`
	PatternHint          string  `json:"pattern_hint"`
	HasAmbiguity         bool    `json:"has_ambiguity"`
	HasExceptions        bool    `json:"has_exceptions"`
	IsCompound           bool    `json:"is_compound"`
	IsManualReview       bool    `json:"is_manual_review"`
	IsExceptionKind      bool    `json:"is_exception_kind"`
	HasConditions        bool    `json:"has_conditions"`
	NonDecisionSection   bool    `json:"non_decision_section"`
	Subjective           bool    `json:"subjective"`
	ConflictingModality  bool    `json:"conflicting_modality"`
	HeuristicBand        string  `json:"heuristic_band,omitempty"`
	Kind                 string  `json:"kind,omitempty"`
	SubmissionKind       string  `json:"submission_kind,omitempty"`
}

// LoadGold reads a gold JSON file from disk.
func LoadGold(path string) (GoldSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return GoldSet{}, err
	}
	var g GoldSet
	if err := json.Unmarshal(raw, &g); err != nil {
		return GoldSet{}, fmt.Errorf("parse gold: %w", err)
	}
	if g.SchemaVersion == "" {
		return GoldSet{}, fmt.Errorf("gold schema_version is required")
	}
	if len(g.Examples) == 0 {
		return GoldSet{}, fmt.Errorf("gold has no examples")
	}
	return g, nil
}
