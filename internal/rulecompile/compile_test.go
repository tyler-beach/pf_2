package rulecompile

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOfflineCompileWithOpenGrepGate(t *testing.T) {
	opengrep := os.Getenv("OPENGREP_BIN")
	if opengrep == "" {
		opengrep = "/tmp/opengrep-bin/opengrep"
	}
	if _, err := os.Stat(opengrep); err != nil {
		t.Skip("opengrep binary not available")
	}
	if out, err := exec.Command(opengrep, "--version").CombinedOutput(); err != nil {
		t.Skipf("opengrep not runnable: %v (%s)", err, out)
	}

	root := filepath.Join("..", "..", "testdata", "corpus", "adrs")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("corpus missing: %v", err)
	}
	out := t.TempDir()
	report, err := Compile(Options{
		Root:           root,
		OutDir:         out,
		Provider:       OfflineProvider{},
		OpenGrepBinary: opengrep,
		Limit:          1,
		Modalities:     []string{"must", "must_not", "shall", "shall_not"},
		Repair:         false,
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if report.CompiledTotal != 1 {
		t.Fatalf("compiled_total=%d want 1", report.CompiledTotal)
	}
	if len(report.Items) != 1 {
		t.Fatalf("items=%d", len(report.Items))
	}
	item := report.Items[0]
	if item.Executable {
		t.Fatal("drafts must not be executable")
	}
	if item.Draft.Route != RouteCompileNew {
		t.Fatalf("route=%s", item.Draft.Route)
	}
	if item.Gate.Status != "passed" {
		t.Fatalf("gate=%s errors=%v", item.Gate.Status, item.Gate.Errors)
	}
	if item.Gate.BadFindings < 1 || item.Gate.GoodFindings != 0 {
		t.Fatalf("findings bad=%d good=%d", item.Gate.BadFindings, item.Gate.GoodFindings)
	}
	if _, err := os.Stat(filepath.Join(out, "report.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "candidates.json")); err != nil {
		t.Fatal(err)
	}
}

func TestStructuralValidateBindExisting(t *testing.T) {
	g := structuralValidate(Draft{Route: RouteBindExisting, ExistingRuleIDs: []string{"pathfinder.x"}})
	if g.Status != "not_applicable" || !g.StructuralOK {
		t.Fatalf("%+v", g)
	}
}

func TestCombineConfidenceGateFail(t *testing.T) {
	p, band := combineConfidence(0.9, 0.9, GateResult{Status: "failed"})
	if p >= 0.8 {
		t.Fatalf("expected penalized p, got %f", p)
	}
	if band == "high" {
		t.Fatalf("band=%s", band)
	}
}

func TestCountJSONResults(t *testing.T) {
	raw := []byte(`{"results":[{"check_id":"a"},{"check_id":"b"}],"errors":[]}`)
	if n := countJSONResults(raw); n != 2 {
		t.Fatalf("n=%d", n)
	}
	if n := countJSONResults([]byte(`{"results":[]}`)); n != 0 {
		t.Fatalf("n=%d", n)
	}
}

func TestNormalizeDraftFallback(t *testing.T) {
	d := normalizeDraft(Draft{RuleYAML: "rules:\n  - id: pathfinder.compiled.x\n"})
	if d.Route != RouteCompileNew {
		t.Fatalf("route=%s", d.Route)
	}
	if d.RuleID != "pathfinder.compiled.x" {
		t.Fatalf("id=%s", d.RuleID)
	}
}

func TestOfflineProviderJSON(t *testing.T) {
	raw, _, err := OfflineProvider{}.Complete("unused")
	if err != nil {
		t.Fatal(err)
	}
	var d Draft
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d.Route != RouteCompileNew || d.RuleYAML == "" {
		t.Fatalf("%+v", d)
	}
}
