package calibration

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	AnalyzerPrecisionSchema = "analyzer-precision-v1"
	DefaultAnalyzerPrecision = 0.7
)

// AnalyzerPrecisionTable maps rule IDs to empiric precision from fixtures.
type AnalyzerPrecisionTable struct {
	SchemaVersion    string             `json:"schema_version"`
	DefaultPrecision float64            `json:"default_precision"`
	PrecisionByRuleID map[string]float64 `json:"precision_by_rule_id"`
	Notes            string             `json:"notes,omitempty"`
}

// ParseAnalyzerPrecision unmarshals the analyzer evidence weight table.
func ParseAnalyzerPrecision(raw []byte) (AnalyzerPrecisionTable, error) {
	var t AnalyzerPrecisionTable
	if err := json.Unmarshal(raw, &t); err != nil {
		return AnalyzerPrecisionTable{}, err
	}
	if t.DefaultPrecision <= 0 {
		t.DefaultPrecision = DefaultAnalyzerPrecision
	}
	if t.PrecisionByRuleID == nil {
		t.PrecisionByRuleID = map[string]float64{}
	}
	return t, nil
}

// LoadAnalyzerPrecision reads the table from disk.
func LoadAnalyzerPrecision(path string) (AnalyzerPrecisionTable, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return AnalyzerPrecisionTable{}, err
	}
	return ParseAnalyzerPrecision(raw)
}

// EvidenceWeight returns empiric precision for a rule ID (or default).
func EvidenceWeight(ruleID string, table AnalyzerPrecisionTable) float64 {
	id := strings.TrimSpace(ruleID)
	if id == "" {
		return table.DefaultPrecision
	}
	if v, ok := table.PrecisionByRuleID[id]; ok && v > 0 {
		return clamp01(v)
	}
	// Prefix / family soft match for CKV_AWS_* style IDs.
	for k, v := range table.PrecisionByRuleID {
		if strings.HasPrefix(id, k) || strings.HasPrefix(k, id) {
			return clamp01(v)
		}
	}
	if table.DefaultPrecision > 0 {
		return clamp01(table.DefaultPrecision)
	}
	return DefaultAnalyzerPrecision
}

// DefaultAnalyzerTable returns the embedded fixture-derived table.
func DefaultAnalyzerTable() AnalyzerPrecisionTable {
	t, err := ParseAnalyzerPrecision(embeddedAnalyzerPrecisionJSON)
	if err != nil {
		return AnalyzerPrecisionTable{
			SchemaVersion:     AnalyzerPrecisionSchema,
			DefaultPrecision:  DefaultAnalyzerPrecision,
			PrecisionByRuleID: map[string]float64{},
			Notes:             fmt.Sprintf("embed parse failed: %v", err),
		}
	}
	return t
}
