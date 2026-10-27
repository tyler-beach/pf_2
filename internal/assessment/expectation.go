package assessment

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	expectedVerdictMarker = regexp.MustCompile(`\[expected:[^\]]*\]`)
	expectedVerdictValue  = regexp.MustCompile(`^\[expected:(PASS|FAIL|INDETERMINATE)\]$`)
)

// CheckExpectedVerdict compares a machine-readable assertion in an issue title
// with an assessment result. The boolean is true only when a marker was present.
// Unmarked issue titles are normal assessments; malformed or duplicate markers
// are errors so a regression cannot silently skip its own assertion.
func CheckExpectedVerdict(title string, result Result) (bool, error) {
	markers := expectedVerdictMarker.FindAllString(title, -1)
	if len(markers) == 0 {
		if strings.Contains(strings.ToLower(title), "[expected:") {
			return false, fmt.Errorf("malformed expected-verdict marker; use [expected:PASS], [expected:FAIL], or [expected:INDETERMINATE]")
		}
		return false, nil
	}
	if len(markers) != 1 {
		return false, fmt.Errorf("issue title must contain exactly one expected-verdict marker; found %d", len(markers))
	}
	parts := expectedVerdictValue.FindStringSubmatch(markers[0])
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid expected-verdict marker %q; use [expected:PASS], [expected:FAIL], or [expected:INDETERMINATE]", markers[0])
	}
	expected := parts[1]
	actual := strings.ToUpper(strings.TrimSpace(result.Verdict))
	switch actual {
	case "PASS", "FAIL", "INDETERMINATE":
	default:
		return true, fmt.Errorf("assessment contains unsupported verdict %q", result.Verdict)
	}
	if actual != expected {
		return true, fmt.Errorf("expected %s verdict, got %s (coverage=%s)", expected, actual, result.CoverageStatus)
	}
	return true, nil
}
