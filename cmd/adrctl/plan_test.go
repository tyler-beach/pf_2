package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func planFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"src/app.py": "print('safe')\n", "rules/security.yml": "rules: []\n"}
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil { t.Fatal(err) }
		if err := os.WriteFile(full, []byte(body), 0600); err != nil { t.Fatal(err) }
	}
	return root
}

func TestPlanRepoCLIIsDeterministicAndVerifyDetectsDrift(t *testing.T) {
	root := planFixtureRoot(t)
	output := filepath.Join(root, "analyzer-evidence", "plan.json")
	args := []string{"--root", root, "--analyzer", "opengrep", "--version", "1.30.0",
		"--config", "rules/security.yml", "--scope", "src/", "--json-out", output}
	if err := runPlanRepo(args); err != nil { t.Fatal(err) }
	first, err := os.ReadFile(output)
	if err != nil { t.Fatal(err) }
	if err := runPlanRepo(args); err != nil { t.Fatal(err) }
	second, err := os.ReadFile(output)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(first, second) { t.Fatal("plan-repo CLI output is not deterministic") }
	if err := runVerifyPlan([]string{"--root", root, "--manifest", output}); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "src", "app.py"), []byte("print('changed')\n"), 0600); err != nil { t.Fatal(err) }
	if err := runVerifyPlan([]string{"--root", root, "--manifest", output}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("verify-plan should reject source drift, got %v", err)
	}
}
