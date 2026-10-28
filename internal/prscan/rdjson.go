package prscan

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

// SupportedReporters are the reviewdog reporters Pathfinder will run when
// selected in the issue template. local writes artifacts only.
var SupportedReporters = []string{
	"github-pr-review",
	"github-pr-check",
	"github-check",
	"github-pr-annotations",
	"local",
}

// ConvertOptions controls assessment → rdjson / security SARIF mapping.
type ConvertOptions struct {
	// MaxInline caps high-signal inline diagnostics (0 = default 25).
	MaxInline int
	// SecurityUncapped when true uploads all deviation findings to Security
	// (still N/A-free). When false, Security uses the same capped set as reviewdog.
	SecurityUncapped bool
	// DocsBase overrides the "Next" docs link.
	DocsBase string
	// SourceName appears in rdjson source.name.
	SourceName string
	// SourceURL appears in rdjson source.url / SARIF informationUri.
	SourceURL string
	// PRURL / HeadSHA / IssueURL / Marker feed the summary sidecar.
	PRURL    string
	HeadSHA  string
	IssueURL string
	Marker   string
	// RunID / WorkshopBase feed Pages dig-in links (#/runs?run_id=…, #/submissions).
	RunID        string
	WorkshopBase string
}

const defaultMaxInline = 25

// RDJSON is reviewdog's DiagnosticResult wire format (rdjson).
type RDJSON struct {
	Source      *RDSource      `json:"source,omitempty"`
	Severity    string         `json:"severity,omitempty"`
	Diagnostics []RDDiagnostic `json:"diagnostics"`
}

// RDSource identifies the producer.
type RDSource struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// RDDiagnostic is one reviewdog finding.
type RDDiagnostic struct {
	Message  string     `json:"message"`
	Location RDLocation `json:"location"`
	Severity string     `json:"severity,omitempty"`
	Code     *RDCode    `json:"code,omitempty"`
}

// RDLocation points at a file/line for inline comments.
type RDLocation struct {
	Path  string   `json:"path"`
	Range *RDRange `json:"range,omitempty"`
}

// RDRange is 1-indexed line/column.
type RDRange struct {
	Start RDPosition  `json:"start"`
	End   *RDPosition `json:"end,omitempty"`
}

// RDPosition is a cursor in a file.
type RDPosition struct {
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
}

// RDCode carries the requirement / rule id.
type RDCode struct {
	Value string `json:"value,omitempty"`
	URL   string `json:"url,omitempty"`
}

// ConvertResult is rdjson + security SARIF + human summary.
type ConvertResult struct {
	Findings      []Finding
	RDJSON        RDJSON
	SecuritySARIF SecuritySARIF
	Summary       Summary
}

// ConvertAssessment maps a Pathfinder assessment into sparse, high-signal
// reviewdog rdjson and GitHub code-scanning SARIF from one shared finding set.
//
// Grill survivors encoded here:
//   - Never emit NOT_APPLICABLE / PASS as diagnostics (no N/A flood).
//   - Only FAIL requirements with at least one finding that has a path.
//   - Dedupe by fingerprint or (path, line, rule, requirement).
//   - Cap inline comments; overflow counted in Summary.
//   - Shared FormatFinding message for every reporter + Security message text.
func ConvertAssessment(result assessment.Result, opt ConvertOptions) ConvertResult {
	if opt.MaxInline <= 0 {
		opt.MaxInline = defaultMaxInline
	}
	if opt.DocsBase == "" {
		opt.DocsBase = DefaultDocsBase
	}
	if opt.SourceName == "" {
		opt.SourceName = "pathfinder"
	}
	if opt.SourceURL == "" {
		opt.SourceURL = "https://github.com/tyler-beach/pf_2"
	}

	all, stats := ExtractFindings(result, opt)
	inline := SelectFindings(all, opt.MaxInline)
	stats.Posted = len(inline)

	securitySet := inline
	if opt.SecurityUncapped {
		securitySet = all
	}

	diags := make([]RDDiagnostic, 0, len(inline))
	for _, f := range inline {
		d := RDDiagnostic{
			Message:  f.Message,
			Severity: f.Severity,
			Location: RDLocation{Path: f.Path},
			Code: &RDCode{
				Value: firstNonEmpty(f.RequirementID, f.RuleID),
				URL:   f.HelpURI,
			},
		}
		if f.Line > 0 {
			start := RDPosition{Line: f.Line}
			if f.Column > 0 {
				start.Column = f.Column
			}
			d.Location.Range = &RDRange{Start: start}
		}
		diags = append(diags, d)
	}

	overall := "INFO"
	for _, d := range diags {
		if severityRank(d.Severity) < severityRank(overall) {
			overall = d.Severity
		}
	}
	if len(diags) == 0 {
		overall = "INFO"
	}

	rd := RDJSON{
		Source:      &RDSource{Name: opt.SourceName, URL: opt.SourceURL},
		Severity:    overall,
		Diagnostics: diags,
	}
	sarif := BuildSecuritySARIF(securitySet, opt)

	return ConvertResult{
		Findings:      all,
		RDJSON:        rd,
		SecuritySARIF: sarif,
		Summary:       stats,
	}
}

// MarshalRDJSON returns pretty-printed rdjson bytes.
func MarshalRDJSON(r RDJSON) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func severityRank(sev string) int {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "ERROR":
		return 0
	case "WARNING":
		return 1
	case "INFO":
		return 2
	default:
		return 3
	}
}

func dedupeKey(req assessment.RequirementResult, f assessment.Finding) string {
	if fp := strings.TrimSpace(f.Fingerprint); fp != "" {
		return "fp:" + fp + "|" + req.RequirementID
	}
	return fmt.Sprintf("%s|%d|%s|%s", f.Path, f.Line, f.RuleID, req.RequirementID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// NormalizeReporters validates and dedupes reporter checkbox values.
func NormalizeReporters(selected []string) ([]string, error) {
	allowed := map[string]struct{}{}
	for _, r := range SupportedReporters {
		allowed[r] = struct{}{}
	}
	out := make([]string, 0, len(selected))
	seen := map[string]struct{}{}
	for _, raw := range selected {
		r := strings.TrimSpace(raw)
		if r == "" {
			continue
		}
		if _, ok := allowed[r]; !ok {
			return nil, fmt.Errorf("unsupported reviewdog reporter %q (supported: %s)", r, strings.Join(SupportedReporters, ", "))
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("select at least one feedback mode")
	}
	return out, nil
}
