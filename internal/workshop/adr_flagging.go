package workshop

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

const ADRFlaggingSchemaVersion = "adr-flagging-v1"

// Alignment classes for ADR flagging analytics.
const (
	AlignmentAligned  = "aligned"
	AlignmentDeviated = "deviated"
	AlignmentExcluded = "excluded"
)

// ClassifyRequirementStatus maps an assessment requirement status into aligned /
// deviate / excluded. Documented in docs/kpi-dictionary.md and workshop docs.
//
// Aligned: PASS (clean applicable check).
// Deviated: FAIL, WARN, INDETERMINATE, EVIDENCE_MISSING, ERROR, REVIEW_REQUIRED, UNSUPPORTED.
// Excluded: NOT_APPLICABLE (and empty) — out of flag-rate denominator.
func ClassifyRequirementStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PASS":
		return AlignmentAligned
	case "NOT_APPLICABLE", "":
		return AlignmentExcluded
	case "FAIL", "WARN", "INDETERMINATE", "EVIDENCE_MISSING", "ERROR", "REVIEW_REQUIRED", "UNSUPPORTED":
		return AlignmentDeviated
	default:
		// Unknown statuses are treated as deviate so they surface for review.
		return AlignmentDeviated
	}
}

// ADRFlaggingObservation is one requirement result attributed to a run/submission.
type ADRFlaggingObservation struct {
	RunID            string              `json:"run_id,omitempty"`
	SubmissionID     string              `json:"submission_id,omitempty"`
	SubmissionKind   string              `json:"submission_kind,omitempty"`
	RequirementID    string              `json:"requirement_id"`
	Status           string              `json:"status"`
	Alignment        string              `json:"alignment"`
	Title            string              `json:"title,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	Rules            []ADRFlaggingRule   `json:"rules,omitempty"`
}

// ADRFlaggingRule captures analyzer/rule/pattern evidence that fired or was bound.
type ADRFlaggingRule struct {
	RuleID   string `json:"rule_id,omitempty"`
	Analyzer string `json:"analyzer,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message,omitempty"`
}

// ADRFlaggingCounts is a status histogram.
type ADRFlaggingCounts map[string]int

// ADRFlaggingKindRollup is per-submission_kind aggregates for one ADR.
type ADRFlaggingKindRollup struct {
	Counts    ADRFlaggingCounts `json:"counts"`
	Assessed  int               `json:"assessed"`
	Aligned   int               `json:"aligned"`
	Deviated  int               `json:"deviated"`
	Excluded  int               `json:"excluded"`
	FlagRate  *float64          `json:"flag_rate,omitempty"`
	FlagRateLo *float64         `json:"flag_rate_ci_low,omitempty"`
	FlagRateHi *float64         `json:"flag_rate_ci_high,omitempty"`
}

// ADRFlaggingRow is the per-ADR rollup used by leaderboards and dig-in.
type ADRFlaggingRow struct {
	ADRID          string                           `json:"adr_id"`
	Title          string                           `json:"title,omitempty"`
	Counts         ADRFlaggingCounts                `json:"counts"`
	Assessed       int                              `json:"assessed"`
	Aligned        int                              `json:"aligned"`
	Deviated       int                              `json:"deviated"`
	Excluded       int                              `json:"excluded"`
	FailCount      int                              `json:"fail_count"`
	Indeterminate  int                              `json:"indeterminate_count"`
	FlagRate       *float64                         `json:"flag_rate,omitempty"`
	FlagRateLo     *float64                         `json:"flag_rate_ci_low,omitempty"`
	FlagRateHi     *float64                         `json:"flag_rate_ci_high,omitempty"`
	ByKind         map[string]ADRFlaggingKindRollup `json:"by_submission_kind,omitempty"`
	Rules          []ADRFlaggingRule                `json:"rules,omitempty"`
	AlignedRuns    []ADRFlaggingObservation         `json:"aligned_runs,omitempty"`
	DeviatedRuns   []ADRFlaggingObservation         `json:"deviated_runs,omitempty"`
	RunIDs         []string                         `json:"run_ids,omitempty"`
}

// ADRFlaggingSummary is the KPI strip payload.
type ADRFlaggingSummary struct {
	ADRTouched           int      `json:"adr_touched"`
	ADRAssessed          int      `json:"adr_assessed"`
	RequirementObs       int      `json:"requirement_observations"`
	AssessedObs          int      `json:"assessed_observations"`
	AlignedObs           int      `json:"aligned_observations"`
	DeviatedObs          int      `json:"deviated_observations"`
	ExcludedObs          int      `json:"excluded_observations"`
	FailObs              int      `json:"fail_observations"`
	IndeterminateObs     int      `json:"indeterminate_observations"`
	FlagRate             *float64 `json:"flag_rate,omitempty"`
	FlagRateLo           *float64 `json:"flag_rate_ci_low,omitempty"`
	FlagRateHi           *float64 `json:"flag_rate_ci_high,omitempty"`
	TopFlagRate          *float64 `json:"top_flag_rate,omitempty"`
	TopFlagRateADR       string   `json:"top_flag_rate_adr,omitempty"`
	MostFailADR          string   `json:"most_fail_adr,omitempty"`
	MostFailCount        int      `json:"most_fail_count,omitempty"`
	MostAlignedADR       string   `json:"most_aligned_adr,omitempty"`
	MostAlignedCount     int      `json:"most_aligned_count,omitempty"`
	MostIndeterminateADR string   `json:"most_indeterminate_adr,omitempty"`
	MostIndeterminateCount int    `json:"most_indeterminate_count,omitempty"`
}

// ADRFlaggingLeaderboards are sorted ADR id slices for the dashboard.
type ADRFlaggingLeaderboards struct {
	MostFlagging       []string `json:"most_flagging"`
	LeastFlagging      []string `json:"least_flagging"`
	MostFail           []string `json:"most_fail"`
	MostIndeterminate  []string `json:"most_indeterminate"`
	MostAligned        []string `json:"most_aligned"`
}

// ADRFlaggingReport is the static JSON artifact for Pages (`adr_flagging.json`).
type ADRFlaggingReport struct {
	SchemaVersion string                  `json:"schema_version"`
	RunID         string                  `json:"run_id,omitempty"`
	Definitions   map[string]string       `json:"definitions"`
	Summary       ADRFlaggingSummary      `json:"summary"`
	Leaderboards  ADRFlaggingLeaderboards `json:"leaderboards"`
	ADRs          []ADRFlaggingRow        `json:"adrs"`
}

// ADRFlaggingInputRequirement is the subset of assessment.RequirementResult we need.
type ADRFlaggingInputRequirement struct {
	ADRID         string
	Title         string
	RequirementID string
	Status        string
	Reason        string
	Findings      []ADRFlaggingRule
}

// ADRFlaggingInput is one assessment (or submission) contributing observations.
type ADRFlaggingInput struct {
	RunID          string
	SubmissionID   string
	SubmissionKind string
	Requirements   []ADRFlaggingInputRequirement
}

const maxStoredObservationsPerADR = 40

// BuildADRFlagging aggregates requirement results across assessments into a rollup.
func BuildADRFlagging(runID string, inputs []ADRFlaggingInput) ADRFlaggingReport {
	type accum struct {
		row      ADRFlaggingRow
		kind     map[string]*ADRFlaggingKindRollup
		rules    map[string]ADRFlaggingRule
		runSet   map[string]struct{}
		aligned  []ADRFlaggingObservation
		deviated []ADRFlaggingObservation
	}
	byADR := map[string]*accum{}

	ensure := func(adrID, title string) *accum {
		a := byADR[adrID]
		if a != nil {
			if a.row.Title == "" && title != "" {
				a.row.Title = title
			}
			return a
		}
		a = &accum{
			row: ADRFlaggingRow{
				ADRID:  adrID,
				Title:  title,
				Counts: ADRFlaggingCounts{},
			},
			kind:   map[string]*ADRFlaggingKindRollup{},
			rules:  map[string]ADRFlaggingRule{},
			runSet: map[string]struct{}{},
		}
		byADR[adrID] = a
		return a
	}

	bumpKind := func(a *accum, kind, status, alignment string) {
		if kind == "" {
			kind = "unspecified"
		}
		k := a.kind[kind]
		if k == nil {
			k = &ADRFlaggingKindRollup{Counts: ADRFlaggingCounts{}}
			a.kind[kind] = k
		}
		k.Counts[status]++
		switch alignment {
		case AlignmentAligned:
			k.Aligned++
			k.Assessed++
		case AlignmentDeviated:
			k.Deviated++
			k.Assessed++
		default:
			k.Excluded++
		}
	}

	for _, in := range inputs {
		for _, req := range in.Requirements {
			adrID := strings.TrimSpace(req.ADRID)
			if adrID == "" {
				continue
			}
			status := strings.ToUpper(strings.TrimSpace(req.Status))
			if status == "" {
				status = "UNKNOWN"
			}
			alignment := ClassifyRequirementStatus(status)
			a := ensure(adrID, req.Title)
			a.row.Counts[status]++
			switch alignment {
			case AlignmentAligned:
				a.row.Aligned++
				a.row.Assessed++
			case AlignmentDeviated:
				a.row.Deviated++
				a.row.Assessed++
			default:
				a.row.Excluded++
			}
			if status == "FAIL" {
				a.row.FailCount++
			}
			if status == "INDETERMINATE" {
				a.row.Indeterminate++
			}
			bumpKind(a, in.SubmissionKind, status, alignment)

			runKey := in.RunID
			if in.SubmissionID != "" {
				runKey = in.SubmissionID
			}
			if runKey != "" {
				a.runSet[runKey] = struct{}{}
			}

			obs := ADRFlaggingObservation{
				RunID:          in.RunID,
				SubmissionID:   in.SubmissionID,
				SubmissionKind: in.SubmissionKind,
				RequirementID:  req.RequirementID,
				Status:         status,
				Alignment:      alignment,
				Title:          req.Title,
				Reason:         req.Reason,
				Rules:          req.Findings,
			}
			for _, rule := range req.Findings {
				key := strings.Join([]string{rule.Analyzer, rule.RuleID, rule.Pattern}, "|")
				if key == "||" {
					continue
				}
				if _, ok := a.rules[key]; !ok {
					a.rules[key] = rule
				}
			}
			switch alignment {
			case AlignmentAligned:
				if len(a.aligned) < maxStoredObservationsPerADR {
					a.aligned = append(a.aligned, obs)
				}
			case AlignmentDeviated:
				if len(a.deviated) < maxStoredObservationsPerADR {
					a.deviated = append(a.deviated, obs)
				}
			}
		}
	}

	rows := make([]ADRFlaggingRow, 0, len(byADR))
	for _, a := range byADR {
		finalizeRates(&a.row)
		a.row.ByKind = map[string]ADRFlaggingKindRollup{}
		for kind, k := range a.kind {
			finalizeKindRates(k)
			a.row.ByKind[kind] = *k
		}
		rules := make([]ADRFlaggingRule, 0, len(a.rules))
		for _, r := range a.rules {
			rules = append(rules, r)
		}
		sort.Slice(rules, func(i, j int) bool {
			if rules[i].Analyzer != rules[j].Analyzer {
				return rules[i].Analyzer < rules[j].Analyzer
			}
			return rules[i].RuleID < rules[j].RuleID
		})
		a.row.Rules = rules
		a.row.AlignedRuns = a.aligned
		a.row.DeviatedRuns = a.deviated
		runIDs := make([]string, 0, len(a.runSet))
		for id := range a.runSet {
			runIDs = append(runIDs, id)
		}
		sort.Strings(runIDs)
		a.row.RunIDs = runIDs
		rows = append(rows, a.row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ADRID < rows[j].ADRID })

	summary := summarizeADRFlagging(rows)
	boards := buildADRLeaderboards(rows, 25)

	return ADRFlaggingReport{
		SchemaVersion: ADRFlaggingSchemaVersion,
		RunID:         runID,
		Definitions: map[string]string{
			"aligned":   "Requirement status PASS — applicable check succeeded with evidence.",
			"deviated":  "Requirement status FAIL, WARN, INDETERMINATE, EVIDENCE_MISSING, ERROR, REVIEW_REQUIRED, or UNSUPPORTED.",
			"excluded":  "Requirement status NOT_APPLICABLE (or empty) — out of flag-rate denominator.",
			"assessed":  "aligned + deviate observations (excludes NOT_APPLICABLE).",
			"flag_rate": "deviated / assessed = (FAIL + other deviate statuses) / assessed. Wilson 95% CI when assessed > 0.",
		},
		Summary:      summary,
		Leaderboards: boards,
		ADRs:         rows,
	}
}

func finalizeRates(row *ADRFlaggingRow) {
	if row.Assessed <= 0 {
		return
	}
	rate := float64(row.Deviated) / float64(row.Assessed)
	row.FlagRate = &rate
	ci := calibration.WilsonInterval(row.Deviated, row.Assessed, 0.95)
	row.FlagRateLo = floatPtr(ci.Low)
	row.FlagRateHi = floatPtr(ci.High)
}

func finalizeKindRates(k *ADRFlaggingKindRollup) {
	if k.Assessed <= 0 {
		return
	}
	rate := float64(k.Deviated) / float64(k.Assessed)
	k.FlagRate = &rate
	ci := calibration.WilsonInterval(k.Deviated, k.Assessed, 0.95)
	k.FlagRateLo = floatPtr(ci.Low)
	k.FlagRateHi = floatPtr(ci.High)
}

func summarizeADRFlagging(rows []ADRFlaggingRow) ADRFlaggingSummary {
	s := ADRFlaggingSummary{ADRTouched: len(rows)}
	var topRate float64
	var topADR string
	var mostFail, mostAligned, mostIndet string
	var mostFailN, mostAlignedN, mostIndetN int
	for _, r := range rows {
		s.RequirementObs += r.Assessed + r.Excluded
		s.AssessedObs += r.Assessed
		s.AlignedObs += r.Aligned
		s.DeviatedObs += r.Deviated
		s.ExcludedObs += r.Excluded
		s.FailObs += r.FailCount
		s.IndeterminateObs += r.Indeterminate
		if r.Assessed > 0 {
			s.ADRAssessed++
		}
		if r.FlagRate != nil && (topADR == "" || *r.FlagRate > topRate || (*r.FlagRate == topRate && r.ADRID < topADR)) {
			topRate = *r.FlagRate
			topADR = r.ADRID
		}
		if r.FailCount > mostFailN || (r.FailCount == mostFailN && r.FailCount > 0 && (mostFail == "" || r.ADRID < mostFail)) {
			mostFailN = r.FailCount
			mostFail = r.ADRID
		}
		if r.Aligned > mostAlignedN || (r.Aligned == mostAlignedN && r.Aligned > 0 && (mostAligned == "" || r.ADRID < mostAligned)) {
			mostAlignedN = r.Aligned
			mostAligned = r.ADRID
		}
		if r.Indeterminate > mostIndetN || (r.Indeterminate == mostIndetN && r.Indeterminate > 0 && (mostIndet == "" || r.ADRID < mostIndet)) {
			mostIndetN = r.Indeterminate
			mostIndet = r.ADRID
		}
	}
	if s.AssessedObs > 0 {
		rate := float64(s.DeviatedObs) / float64(s.AssessedObs)
		s.FlagRate = &rate
		ci := calibration.WilsonInterval(s.DeviatedObs, s.AssessedObs, 0.95)
		s.FlagRateLo = floatPtr(ci.Low)
		s.FlagRateHi = floatPtr(ci.High)
	}
	if topADR != "" {
		s.TopFlagRate = &topRate
		s.TopFlagRateADR = topADR
	}
	s.MostFailADR = mostFail
	s.MostFailCount = mostFailN
	s.MostAlignedADR = mostAligned
	s.MostAlignedCount = mostAlignedN
	s.MostIndeterminateADR = mostIndet
	s.MostIndeterminateCount = mostIndetN
	return s
}

func buildADRLeaderboards(rows []ADRFlaggingRow, limit int) ADRFlaggingLeaderboards {
	assessed := make([]ADRFlaggingRow, 0, len(rows))
	for _, r := range rows {
		if r.Assessed > 0 {
			assessed = append(assessed, r)
		}
	}
	ids := func(list []ADRFlaggingRow, less func(a, b ADRFlaggingRow) bool) []string {
		cp := append([]ADRFlaggingRow(nil), list...)
		sort.SliceStable(cp, func(i, j int) bool { return less(cp[i], cp[j]) })
		out := make([]string, 0, limit)
		for i, r := range cp {
			if i >= limit {
				break
			}
			out = append(out, r.ADRID)
		}
		return out
	}
	flagRate := func(r ADRFlaggingRow) float64 {
		if r.FlagRate == nil {
			return -1
		}
		return *r.FlagRate
	}
	boards := ADRFlaggingLeaderboards{
		MostFlagging: ids(assessed, func(a, b ADRFlaggingRow) bool {
			if flagRate(a) != flagRate(b) {
				return flagRate(a) > flagRate(b)
			}
			if a.Deviated != b.Deviated {
				return a.Deviated > b.Deviated
			}
			return a.ADRID < b.ADRID
		}),
		LeastFlagging: ids(assessed, func(a, b ADRFlaggingRow) bool {
			ra, rb := flagRate(a), flagRate(b)
			if ra < 0 {
				ra = 0
			}
			if rb < 0 {
				rb = 0
			}
			if ra != rb {
				return ra < rb
			}
			if a.Aligned != b.Aligned {
				return a.Aligned > b.Aligned
			}
			return a.ADRID < b.ADRID
		}),
		MostFail: ids(rows, func(a, b ADRFlaggingRow) bool {
			if a.FailCount != b.FailCount {
				return a.FailCount > b.FailCount
			}
			return a.ADRID < b.ADRID
		}),
		MostIndeterminate: ids(rows, func(a, b ADRFlaggingRow) bool {
			if a.Indeterminate != b.Indeterminate {
				return a.Indeterminate > b.Indeterminate
			}
			return a.ADRID < b.ADRID
		}),
		MostAligned: ids(rows, func(a, b ADRFlaggingRow) bool {
			if a.Aligned != b.Aligned {
				return a.Aligned > b.Aligned
			}
			return a.ADRID < b.ADRID
		}),
	}
	return boards
}

// ApplyADRFlaggingMetrics copies summary fields onto Metrics.
func ApplyADRFlaggingMetrics(m *Metrics, report ADRFlaggingReport) {
	if m == nil {
		return
	}
	m.ADRTouched = intPtr(report.Summary.ADRTouched)
	m.ADRAssessed = intPtr(report.Summary.ADRAssessed)
	m.ADRFailCount = intPtr(report.Summary.FailObs)
	m.ADRPassCount = intPtr(report.Summary.AlignedObs)
	m.ADRIndeterminateCount = intPtr(report.Summary.IndeterminateObs)
	m.ADRFlagRate = report.Summary.FlagRate
	m.ADRFlagRateLo = report.Summary.FlagRateLo
	m.ADRFlagRateHi = report.Summary.FlagRateHi
	m.ADRTopFlagRate = report.Summary.TopFlagRate
	m.ADRTopFlagRateADR = report.Summary.TopFlagRateADR
	m.ADRMostFail = report.Summary.MostFailADR
	m.ADRMostAligned = report.Summary.MostAlignedADR
}

// FormatADRFlaggingHelp returns a short UI/docs blurb.
func FormatADRFlaggingHelp() string {
	return fmt.Sprintf("aligned=PASS; deviate=FAIL|WARN|INDETERMINATE|EVIDENCE_MISSING|ERROR|REVIEW_REQUIRED|UNSUPPORTED; excluded=NOT_APPLICABLE; flag_rate=deviated/assessed")
}

// LoadADRFlaggingInputsFromSubmissions reads a workshop submissions index and
// each referenced assessment.json into flagging inputs.
func LoadADRFlaggingInputsFromSubmissions(indexPath, runID string) ([]ADRFlaggingInput, error) {
	var index struct {
		RunID       string `json:"run_id"`
		Submissions []struct {
			ID             string `json:"id"`
			SubmissionKind string `json:"submission_kind"`
			AssessmentPath string `json:"assessment_path"`
		} `json:"submissions"`
	}
	if err := readJSON(indexPath, &index); err != nil {
		return nil, err
	}
	if runID == "" {
		runID = index.RunID
	}
	base := filepath.Dir(indexPath)
	out := make([]ADRFlaggingInput, 0, len(index.Submissions))
	for _, sub := range index.Submissions {
		path := sub.AssessmentPath
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			// Prefer paths relative to repo root when the index lives under results/.
			candidates := []string{
				path,
				filepath.Join(base, path),
				filepath.Join(base, sub.ID, "assessment.json"),
			}
			found := ""
			for _, c := range candidates {
				if _, err := os.Stat(c); err == nil {
					found = c
					break
				}
			}
			if found == "" {
				return nil, fmt.Errorf("assessment for %s not found (tried %q)", sub.ID, path)
			}
			path = found
		}
		var assessment struct {
			SubmissionKind string `json:"submission_kind"`
			Requirements   []struct {
				ADRID         string `json:"adr_id"`
				Title         string `json:"title"`
				RequirementID string `json:"requirement_id"`
				Status        string `json:"status"`
				Reason        string `json:"reason"`
				Findings      []struct {
					RuleID   string `json:"rule_id"`
					Analyzer string `json:"analyzer"`
					Path     string `json:"path"`
					Message  string `json:"message"`
				} `json:"findings"`
			} `json:"requirements"`
		}
		if err := readJSON(path, &assessment); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		kind := sub.SubmissionKind
		if kind == "" {
			kind = assessment.SubmissionKind
		}
		reqs := make([]ADRFlaggingInputRequirement, 0, len(assessment.Requirements))
		for _, r := range assessment.Requirements {
			findings := make([]ADRFlaggingRule, 0, len(r.Findings))
			for _, f := range r.Findings {
				findings = append(findings, ADRFlaggingRule{
					RuleID: f.RuleID, Analyzer: f.Analyzer, Path: f.Path, Message: f.Message,
				})
			}
			reqs = append(reqs, ADRFlaggingInputRequirement{
				ADRID: r.ADRID, Title: r.Title, RequirementID: r.RequirementID,
				Status: r.Status, Reason: r.Reason, Findings: findings,
			})
		}
		out = append(out, ADRFlaggingInput{
			RunID: runID, SubmissionID: sub.ID, SubmissionKind: kind, Requirements: reqs,
		})
	}
	return out, nil
}

func writeADRFlaggingArtifact(outDir string, metrics *Metrics, links, copied map[string]string, report ADRFlaggingReport) error {
	ApplyADRFlaggingMetrics(metrics, report)
	if err := writeJSON(filepath.Join(outDir, "adr_flagging.json"), report); err != nil {
		return err
	}
	copied["adr_flagging"] = "adr_flagging.json"
	links["adr_flagging"] = "adr_flagging.json"
	return nil
}
