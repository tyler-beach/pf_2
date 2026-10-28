package calibration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

// TestFitAndWriteParams regenerates committed params when PF_WRITE_CALIBRATION=1.
func TestFitAndWriteParams(t *testing.T) {
	if os.Getenv("PF_WRITE_CALIBRATION") != "1" {
		t.Skip("set PF_WRITE_CALIBRATION=1 to rewrite params-v1.json")
	}
	root := repoRoot(t)
	gold, err := calibration.LoadGold(filepath.Join(root, "testdata/confidence/gold-extract-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	ap, err := calibration.LoadAnalyzerPrecision(filepath.Join(root, "internal/obligations/calibration/analyzer-precision-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := calibration.FitFromGold(gold, ap.PrecisionByRuleID)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(root, "internal/obligations/calibration/params-v1.json"),
		filepath.Join(root, "testdata/confidence/calibration-v1.json"),
	}
	for _, path := range paths {
		if err := calibration.WriteParamsJSON(path, p); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s method=%s n=%d q=%v", path, p.Method, p.GoldExampleCount, p.ConformalQuantile)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// package dir → repo root
	return filepath.Clean(filepath.Join(wd, "../../.."))
}
