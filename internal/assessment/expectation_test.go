package assessment

import (
	"strings"
	"testing"
)

func TestCheckExpectedVerdict(t *testing.T) {
	cases := []struct {
		name, title, actual, coverage string
		matched, wantErr bool
	}{
		{"unmarked", "[ADR Assessment] normal", "PASS", "COMPLETE", false, false},
		{"pass", "[ADR Assessment] [expected:PASS] clean", "PASS", "PARTIAL", true, false},
		{"fail", "[ADR Assessment] [expected:FAIL] violation", "FAIL", "COMPLETE", true, false},
		{"indeterminate", "[ADR Assessment] [expected:INDETERMINATE] missing", "INDETERMINATE", "PARTIAL", true, false},
		{"mismatch", "[ADR Assessment] [expected:PASS] missing", "INDETERMINATE", "PARTIAL", true, true},
		{"bad case", "[ADR Assessment] [expected:pass]", "PASS", "COMPLETE", false, true},
		{"unknown expected", "[ADR Assessment] [expected:REVIEW]", "PASS", "COMPLETE", false, true},
		{"duplicate", "[ADR Assessment] [expected:PASS] [expected:FAIL]", "PASS", "COMPLETE", false, true},
		{"malformed", "[ADR Assessment] [expected:]", "PASS", "COMPLETE", false, true},
		{"unsupported actual", "[ADR Assessment] [expected:PASS]", "UNKNOWN", "COMPLETE", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := CheckExpectedVerdict(tc.title, Result{Verdict:tc.actual, CoverageStatus:tc.coverage})
			if matched != tc.matched { t.Errorf("matched=%v, want %v", matched, tc.matched) }
			if (err != nil) != tc.wantErr { t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr) }
			if err != nil && tc.name == "mismatch" && !strings.Contains(err.Error(), "expected PASS") {
				t.Errorf("mismatch error is not actionable: %v", err)
			}
		})
	}
}
