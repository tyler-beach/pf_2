package prscan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

// Finding is the shared Pathfinder deviation model consumed by both the
// reviewdog (rdjson) formatter and the GitHub code-scanning (SARIF) formatter.
// Only FAIL deviations with a file path become findings — never N/A/PASS noise.
type Finding struct {
	ADRID         string
	ADRVersion    int
	RequirementID string
	Title         string
	Severity      string // ERROR | WARNING | INFO
	RuleID        string // stable across runs for Security dedupe
	Path          string
	Line          int
	Column        int
	Why           string
	Reason        string
	Analyzer      string
	Fingerprint   string // stable partialFingerprint seed
	HelpURI       string
	Message       string // shared formatted body (severity/ADR/why/evidence/next)
}

// ExtractFindings pulls high-signal FAIL deviations from an assessment.
// Dedupes by fingerprint/(path,line,rule,requirement). Does not apply the
// inline cap — callers cap for reviewdog; Security SARIF uses the same set
// unless MaxInline is applied via SelectFindings.
func ExtractFindings(result assessment.Result, opt ConvertOptions) (findings []Finding, stats Summary) {
	if opt.DocsBase == "" {
		opt.DocsBase = DefaultDocsBase
	}

	var (
		na, pass, other, failWithPath int
		seen                          = map[string]struct{}{}
		deduped                       int
		out                           []Finding
	)

	for _, req := range result.Requirements {
		switch strings.ToUpper(strings.TrimSpace(req.Status)) {
		case "NOT_APPLICABLE":
			na++
			continue
		case "PASS":
			pass++
			continue
		case "FAIL":
		default:
			other++
			continue
		}
		if len(req.Findings) == 0 {
			other++
			continue
		}
		for _, f := range req.Findings {
			path := strings.TrimSpace(f.Path)
			if path == "" {
				continue
			}
			failWithPath++
			key := dedupeKey(req, f)
			if _, ok := seen[key]; ok {
				deduped++
				continue
			}
			seen[key] = struct{}{}

			sev := mapSeverity(req.Severity, f.Level)
			ruleID := stableRuleID(req, f)
			help := opt.DocsBase
			msg := FormatFinding(req, f, help)
			fp := strings.TrimSpace(f.Fingerprint)
			if fp == "" {
				fp = hashFingerprint(ruleID, path, f.Line, req.RequirementID)
			}
			out = append(out, Finding{
				ADRID:         req.ADR,
				ADRVersion:    req.ADRVersion,
				RequirementID: req.RequirementID,
				Title:         req.Title,
				Severity:      sev,
				RuleID:        ruleID,
				Path:          path,
				Line:          f.Line,
				Column:        f.Column,
				Why:           firstNonEmpty(strings.TrimSpace(f.Message), compactOneLine(req.Reason)),
				Reason:        req.Reason,
				Analyzer:      f.Analyzer,
				Fingerprint:   fp,
				HelpURI:       help,
				Message:       msg,
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if severityRank(out[i].Severity) != severityRank(out[j].Severity) {
			return severityRank(out[i].Severity) < severityRank(out[j].Severity)
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].RequirementID < out[j].RequirementID
	})

	stats = Summary{
		Verdict:          result.Verdict,
		CoverageStatus:   result.CoverageStatus,
		Applicable:       result.ApplicableRequirementCount,
		RequirementCount: result.RequirementCount,
		Posted:           len(out),
		CappedFrom:       failWithPath,
		NotApplicable:    na,
		Pass:             pass,
		OtherNonFail:     other,
		Deduped:          deduped,
		PRURL:            opt.PRURL,
		HeadSHA:          opt.HeadSHA,
		IssueURL:         opt.IssueURL,
		DocsBase:         opt.DocsBase,
		Marker:           opt.Marker,
	}
	return out, stats
}

// SelectFindings applies MaxInline cap (0 = default 25). Used for reviewdog
// sparse inline comments; Security SARIF may use the uncapped set via
// ConvertOptions.SecurityUncapped.
func SelectFindings(all []Finding, maxInline int) []Finding {
	if maxInline <= 0 {
		maxInline = defaultMaxInline
	}
	if len(all) <= maxInline {
		return all
	}
	return all[:maxInline]
}

func stableRuleID(req assessment.RequirementResult, f assessment.Finding) string {
	// Prefer requirement id (stable ADR binding). Fall back to analyzer rule.
	if id := strings.TrimSpace(req.RequirementID); id != "" {
		return "pathfinder/" + id
	}
	if id := strings.TrimSpace(f.RuleID); id != "" {
		return "pathfinder/" + id
	}
	if adr := strings.TrimSpace(req.ADR); adr != "" {
		return "pathfinder/" + adr
	}
	return "pathfinder/deviation"
}

func hashFingerprint(ruleID, path string, line int, reqID string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s", ruleID, path, line, reqID)))
	return hex.EncodeToString(h[:16])
}
