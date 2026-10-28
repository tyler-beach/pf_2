package prscan

import (
	"fmt"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
)

// DefaultDocsBase is the workshop docs deep-link used in "Next" lines when no
// override is provided. Operators can set ConvertOptions.DocsBase.
const DefaultDocsBase = "https://tyler-beach.github.io/pf_2/#/docs/pr-scan"

// DefaultWorkshopBase is the Pages origin for run/submission dig-in links.
const DefaultWorkshopBase = "https://tyler-beach.github.io/pf_2"

// CommentBlocks is the shared, reporter-agnostic body used for every finding.
// All reviewdog reporters consume the same message string so inline reviews,
// checks, and annotations stay consistent.
type CommentBlocks struct {
	Severity string
	ADRID    string
	Title    string
	Why      string
	Evidence string
	Next     string
}

// FormatComment renders a short, scannable finding body.
//
//	[ERROR] ADR-003 · No dynamic shell
//	Why: Avoid shell invocation
//	Evidence: `internal/run.go:14`
//	Next: Harden the call site — https://…#/docs/repo-ci
func FormatComment(b CommentBlocks) string {
	sev := strings.ToUpper(strings.TrimSpace(b.Severity))
	if sev == "" {
		sev = "ERROR"
	}
	adr := strings.TrimSpace(b.ADRID)
	if adr == "" {
		adr = "ADR-?"
	}
	title := compactOneLine(b.Title)
	if title == "" {
		title = "Requirement deviation"
	}
	why := compactOneLine(b.Why)
	if why == "" {
		why = "Deterministic verifier reported a deviation."
	}
	evidence := strings.TrimSpace(b.Evidence)
	if evidence == "" {
		evidence = "(no path/line)"
	}
	next := compactOneLine(b.Next)
	if next == "" {
		next = "See Pathfinder docs — " + DefaultDocsBase
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] %s · %s\n", sev, adr, title)
	fmt.Fprintf(&sb, "Why: %s\n", why)
	fmt.Fprintf(&sb, "Evidence: `%s`\n", evidence)
	fmt.Fprintf(&sb, "Next: %s", next)
	return sb.String()
}

// FormatFinding builds CommentBlocks from an assessment finding + requirement.
func FormatFinding(req assessment.RequirementResult, f assessment.Finding, docsBase string) string {
	if docsBase == "" {
		docsBase = DefaultDocsBase
	}
	evidence := f.Path
	if f.Line > 0 {
		evidence = fmt.Sprintf("%s:%d", f.Path, f.Line)
	}
	why := strings.TrimSpace(f.Message)
	if why == "" {
		why = compactOneLine(req.Reason)
	}
	next := fmt.Sprintf("Fix deviation for %s — %s", req.RequirementID, docsBase)
	sev := mapSeverity(req.Severity, f.Level)
	return FormatComment(CommentBlocks{
		Severity: sev,
		ADRID:    req.ADR,
		Title:    req.Title,
		Why:      why,
		Evidence: evidence,
		Next:     next,
	})
}

// FormatSummary builds the PR-level summary (posted once; not per-finding spam).
func FormatSummary(s Summary) string {
	var sb strings.Builder
	sb.WriteString("## Pathfinder PR scan\n\n")
	fmt.Fprintf(&sb, "- **Verdict:** `%s` · **Coverage:** `%s` (%d/%d applicable)\n",
		s.Verdict, s.CoverageStatus, s.Applicable, s.RequirementCount)
	fmt.Fprintf(&sb, "- **Inline findings posted:** %d", s.Posted)
	if s.CappedFrom > s.Posted {
		fmt.Fprintf(&sb, " (capped from %d FAIL findings)", s.CappedFrom)
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "- **Noise omitted:** %d NOT_APPLICABLE, %d PASS, %d other non-FAIL\n",
		s.NotApplicable, s.Pass, s.OtherNonFail)
	if s.Deduped > 0 {
		fmt.Fprintf(&sb, "- **Deduped:** %d duplicate finding(s)\n", s.Deduped)
	}
	if s.PRURL != "" {
		fmt.Fprintf(&sb, "- **Target PR:** %s\n", s.PRURL)
	}
	if s.HeadSHA != "" {
		fmt.Fprintf(&sb, "- **Head SHA:** `%s`\n", shortSHA(s.HeadSHA))
	}
	if s.IssueURL != "" {
		fmt.Fprintf(&sb, "- **Scan issue:** %s\n", s.IssueURL)
	}
	if s.DocsBase != "" {
		fmt.Fprintf(&sb, "- **Docs:** %s\n", s.DocsBase)
	}
	workshop := strings.TrimSpace(s.WorkshopBase)
	if workshop == "" {
		workshop = DefaultWorkshopBase
	}
	workshop = strings.TrimRight(workshop, "/")
	if s.RunID != "" {
		fmt.Fprintf(&sb, "- **Workshop run:** %s/#/runs?run_id=%s\n", workshop, s.RunID)
		fmt.Fprintf(&sb, "- **Submissions dig-in:** %s/#/submissions\n", workshop)
	} else if s.WorkshopURL != "" {
		fmt.Fprintf(&sb, "- **Workshop:** %s\n", s.WorkshopURL)
	}
	sb.WriteString("\n> Only FAIL deviations with path/line evidence are posted as reviewdog diagnostics. N/A floods are never inlined.\n")
	if s.Marker != "" {
		fmt.Fprintf(&sb, "\n<!-- pathfinder-pr-scan:%s -->\n", s.Marker)
	}
	return sb.String()
}

// Summary is the rollup used by FormatSummary and emitted beside rdjson.
type Summary struct {
	Verdict          string
	CoverageStatus   string
	Applicable       int
	RequirementCount int
	Posted           int
	CappedFrom       int
	NotApplicable    int
	Pass             int
	OtherNonFail     int
	Deduped          int
	PRURL            string
	HeadSHA          string
	IssueURL         string
	DocsBase         string
	WorkshopBase     string
	WorkshopURL      string
	RunID            string
	Marker           string
}

func mapSeverity(reqSeverity, findingLevel string) string {
	// Prefer ADR/requirement severity over analyzer hit level so reviewdog
	// annotations match obligation severity (OpenGrep often emits "warning"
	// even when the requirement is error — which would leave github-pr-check
	// neutral and unusable for branch protection).
	for _, s := range []string{reqSeverity, findingLevel} {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "error", "err":
			return "ERROR"
		case "warning", "warn":
			return "WARNING"
		case "info", "note", "information":
			return "INFO"
		}
	}
	return "ERROR"
}

func compactOneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
