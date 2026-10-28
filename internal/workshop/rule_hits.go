package workshop

import (
	"path/filepath"
	"sort"
	"strings"
)

const RuleHitsSchemaVersion = "rule-hits-v1"

// RuleHitRow is one analyzer rule_id rollup.
type RuleHitRow struct {
	RuleID     string `json:"rule_id"`
	Analyzer   string `json:"analyzer,omitempty"`
	HitCount   int    `json:"hit_count"`
	FlagCount  int    `json:"flag_count"`
	ADRCount   int    `json:"adr_count,omitempty"`
	SampleADRs []string `json:"sample_adrs,omitempty"`
}

// RuleHitsSummary is the KPI strip for rule/analyzer effectiveness.
type RuleHitsSummary struct {
	DistinctRules int    `json:"distinct_rules"`
	HitTotal      int    `json:"hit_total"`
	FlagTotal     int    `json:"flag_total"`
	TopRuleID     string `json:"top_rule_id,omitempty"`
	TopHitCount   int    `json:"top_hit_count,omitempty"`
	TopFlagRuleID string `json:"top_flag_rule_id,omitempty"`
	TopFlagCount  int    `json:"top_flag_count,omitempty"`
}

// RuleHitsReport is the static artifact for Pages (`rule_hits.json`).
type RuleHitsReport struct {
	SchemaVersion string          `json:"schema_version"`
	RunID         string          `json:"run_id,omitempty"`
	Summary       RuleHitsSummary `json:"summary"`
	Rules         []RuleHitRow    `json:"rules"`
}

type ruleAccum struct {
	row    RuleHitRow
	adrSet map[string]struct{}
}

// BuildRuleHits aggregates OpenGrep/Checkov (etc.) findings from flagging inputs.
// flag_count increments when the parent requirement alignment is deviate.
func BuildRuleHits(runID string, inputs []ADRFlaggingInput) RuleHitsReport {
	byRule := map[string]*ruleAccum{}
	ensure := func(ruleID, analyzer string) *ruleAccum {
		key := ruleID
		a := byRule[key]
		if a != nil {
			if a.row.Analyzer == "" && analyzer != "" {
				a.row.Analyzer = analyzer
			}
			return a
		}
		a = &ruleAccum{
			row:    RuleHitRow{RuleID: ruleID, Analyzer: analyzer},
			adrSet: map[string]struct{}{},
		}
		byRule[key] = a
		return a
	}

	for _, in := range inputs {
		for _, req := range in.Requirements {
			alignment := ClassifyRequirementStatus(req.Status)
			for _, f := range req.Findings {
				ruleID := strings.TrimSpace(f.RuleID)
				if ruleID == "" {
					continue
				}
				a := ensure(ruleID, strings.TrimSpace(f.Analyzer))
				a.row.HitCount++
				if alignment == AlignmentDeviated {
					a.row.FlagCount++
				}
				if req.ADRID != "" {
					a.adrSet[req.ADRID] = struct{}{}
				}
			}
		}
	}

	rows := make([]RuleHitRow, 0, len(byRule))
	sum := RuleHitsSummary{}
	topHits, topFlags := 0, 0
	for _, a := range byRule {
		adrs := make([]string, 0, len(a.adrSet))
		for id := range a.adrSet {
			adrs = append(adrs, id)
		}
		sort.Strings(adrs)
		a.row.ADRCount = len(adrs)
		if len(adrs) > 8 {
			a.row.SampleADRs = adrs[:8]
		} else {
			a.row.SampleADRs = adrs
		}
		rows = append(rows, a.row)
		sum.HitTotal += a.row.HitCount
		sum.FlagTotal += a.row.FlagCount
		if a.row.HitCount > topHits || (a.row.HitCount == topHits && (sum.TopRuleID == "" || a.row.RuleID < sum.TopRuleID)) {
			topHits = a.row.HitCount
			sum.TopRuleID = a.row.RuleID
			sum.TopHitCount = a.row.HitCount
		}
		if a.row.FlagCount > topFlags || (a.row.FlagCount == topFlags && a.row.FlagCount > 0 && (sum.TopFlagRuleID == "" || a.row.RuleID < sum.TopFlagRuleID)) {
			topFlags = a.row.FlagCount
			sum.TopFlagRuleID = a.row.RuleID
			sum.TopFlagCount = a.row.FlagCount
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].HitCount != rows[j].HitCount {
			return rows[i].HitCount > rows[j].HitCount
		}
		if rows[i].FlagCount != rows[j].FlagCount {
			return rows[i].FlagCount > rows[j].FlagCount
		}
		return rows[i].RuleID < rows[j].RuleID
	})
	sum.DistinctRules = len(rows)
	return RuleHitsReport{
		SchemaVersion: RuleHitsSchemaVersion,
		RunID:         runID,
		Summary:       sum,
		Rules:         rows,
	}
}

// ApplyRuleHitsMetrics copies summary fields onto Metrics.
func ApplyRuleHitsMetrics(m *Metrics, report RuleHitsReport) {
	if m == nil {
		return
	}
	m.RuleDistinctCount = intPtr(report.Summary.DistinctRules)
	m.RuleHitTotal = intPtr(report.Summary.HitTotal)
	m.RuleFlagTotal = intPtr(report.Summary.FlagTotal)
	m.RuleTopRuleID = report.Summary.TopRuleID
	if report.Summary.TopHitCount > 0 {
		m.RuleTopHitCount = intPtr(report.Summary.TopHitCount)
	}
	m.RuleTopFlagRuleID = report.Summary.TopFlagRuleID
	if report.Summary.TopFlagCount > 0 {
		m.RuleTopFlagCount = intPtr(report.Summary.TopFlagCount)
	}
}

func writeRuleHitsArtifact(outDir string, metrics *Metrics, links, copied map[string]string, report RuleHitsReport) error {
	if len(report.Rules) == 0 && report.Summary.DistinctRules == 0 {
		return nil
	}
	ApplyRuleHitsMetrics(metrics, report)
	if err := writeJSON(filepath.Join(outDir, "rule_hits.json"), report); err != nil {
		return err
	}
	copied["rule_hits"] = "rule_hits.json"
	links["rule_hits"] = "rule_hits.json"
	return nil
}
