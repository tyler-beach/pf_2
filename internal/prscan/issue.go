package prscan

import (
	"fmt"
	"regexp"
	"strings"
)

// IssueScanRequest is the parsed Pathfinder PR-scan issue form.
type IssueScanRequest struct {
	PRURL       string
	PR          PRRef
	Reporters   []string
	UploadSARIF bool
	FilterMode  string
	Catalog     string // "corpus" or "live"
	FailLevel   string
}

var (
	prURLLine     = regexp.MustCompile(`(?im)^###\s*Pull request URL\s*$`)
	sectionHeader = regexp.MustCompile(`(?m)^###\s+(.+?)\s*$`)
)

// ParseIssueBody extracts fields from a GitHub issue-form body.
// It is intentionally tolerant of checkbox wording and blank lines.
func ParseIssueBody(body string) (IssueScanRequest, error) {
	sections := splitFormSections(body)
	prRaw := firstNonEmptyLine(sections["Pull request URL"])
	if prRaw == "" {
		prRaw = findFirstPRURL(body)
	}
	if prRaw == "" {
		return IssueScanRequest{}, fmt.Errorf("missing Pull request URL")
	}
	ref, err := ParsePRURL(prRaw)
	if err != nil {
		return IssueScanRequest{}, err
	}

	reporters := parseCheckedReporters(sections["Feedback modes (reviewdog)"])
	if len(reporters) == 0 {
		reporters = parseCheckedReporters(sections["Feedback modes"])
	}
	uploadSARIF := parseUploadSARIF(sections["GitHub Security (code scanning)"])
	if !uploadSARIF {
		uploadSARIF = parseUploadSARIF(sections["Security / SARIF"])
	}
	if !uploadSARIF {
		uploadSARIF = bodyHasCheckedSARIF(body)
	}

	if len(reporters) > 0 {
		reporters, err = NormalizeReporters(reporters)
		if err != nil {
			return IssueScanRequest{}, err
		}
	}
	if len(reporters) == 0 && !uploadSARIF {
		return IssueScanRequest{}, fmt.Errorf("select at least one reviewdog feedback mode and/or SARIF Security upload")
	}

	filter := firstNonEmptyLine(sections["Diff filter mode"])
	if filter == "" {
		filter = "added"
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	switch filter {
	case "added", "diff_context", "file", "nofilter":
	default:
		return IssueScanRequest{}, fmt.Errorf("invalid filter mode %q", filter)
	}

	catalog := firstNonEmptyLine(sections["Requirement catalog"])
	catalog = strings.ToLower(strings.TrimSpace(catalog))
	switch {
	case catalog == "" || strings.Contains(catalog, "corpus"):
		catalog = "corpus"
	case strings.Contains(catalog, "live") || strings.Contains(catalog, "adr/requirements"):
		catalog = "live"
	default:
		return IssueScanRequest{}, fmt.Errorf("invalid catalog choice %q", catalog)
	}

	fail := firstNonEmptyLine(sections["Fail level"])
	if fail == "" {
		fail = "none"
	}
	fail = strings.ToLower(strings.TrimSpace(fail))
	switch fail {
	case "none", "any", "info", "warning", "error":
	default:
		return IssueScanRequest{}, fmt.Errorf("invalid fail level %q", fail)
	}

	return IssueScanRequest{
		PRURL:       prRaw,
		PR:          ref,
		Reporters:   reporters,
		UploadSARIF: uploadSARIF,
		FilterMode:  filter,
		Catalog:     catalog,
		FailLevel:   fail,
	}, nil
}

func splitFormSections(body string) map[string]string {
	out := map[string]string{}
	idxs := sectionHeader.FindAllStringSubmatchIndex(body, -1)
	if len(idxs) == 0 {
		return out
	}
	for i, loc := range idxs {
		title := strings.TrimSpace(body[loc[2]:loc[3]])
		start := loc[1]
		end := len(body)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		out[title] = body[start:end]
	}
	return out
}

func firstNonEmptyLine(section string) string {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "_" || line == "<!-- -->" {
			continue
		}
		return line
	}
	return ""
}

func parseCheckedReporters(section string) []string {
	var out []string
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [x]") && !strings.HasPrefix(line, "- [X]") {
			continue
		}
		label := strings.TrimSpace(line[5:])
		if id := extractReporterID(label); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func parseUploadSARIF(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [x]") && !strings.HasPrefix(line, "- [X]") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "sarif") || strings.Contains(lower, "security") || strings.Contains(lower, "code scanning") {
			return true
		}
	}
	return false
}

func bodyHasCheckedSARIF(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [x]") && !strings.HasPrefix(line, "- [X]") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "upload sarif") || strings.Contains(lower, "code scanning") {
			return true
		}
	}
	return false
}

func extractReporterID(label string) string {
	label = strings.TrimSpace(label)
	if i := strings.Index(label, "`"); i >= 0 {
		rest := label[i+1:]
		if j := strings.Index(rest, "`"); j > 0 {
			cand := rest[:j]
			for _, r := range SupportedReporters {
				if cand == r {
					return r
				}
			}
		}
	}
	for _, r := range SupportedReporters {
		if strings.HasPrefix(label, r) || strings.Contains(label, r) {
			return r
		}
	}
	return ""
}

var loosePRURL = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:api\.)?github(?:usercontent)?\.com/[^\s)]+/pull/\d+`)

func findFirstPRURL(body string) string {
	_ = prURLLine
	if m := loosePRURL.FindString(body); m != "" {
		return m
	}
	return ""
}
