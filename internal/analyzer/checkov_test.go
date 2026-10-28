package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const checkovFixtureSARIF = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov","version":"3.3.30","rules":[{"id":"CKV_AWS_24"}]}},"results":[{"ruleId":"CKV_AWS_24","level":"error","message":{"text":"Ensure no security groups allow ingress from 0.0.0.0/0 to port 22"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"security_group.tf"},"region":{"startLine":7,"startColumn":1}}}]}]}]}`

type checkovFakeRunner struct {
	calls         []runnerCall
	sarif         []byte
	scanExitCode  int
	versionOutput []byte
	scanErr       error
}

func (f *checkovFakeRunner) Run(ctx context.Context, binary string, args []string, dir string) (CommandResult, error) {
	f.calls = append(f.calls, runnerCall{binary: binary, args: append([]string(nil), args...), dir: dir})
	if len(args) == 1 && args[0] == "--version" {
		output := f.versionOutput
		if output == nil {
			output = []byte("checkov version: 3.3.30\n")
		}
		return CommandResult{ExitCode: 0, Stdout: output}, nil
	}
	if f.scanErr != nil {
		return CommandResult{ExitCode: -1}, f.scanErr
	}
	var outDir string
	for i, arg := range args {
		if arg == "--output-file-path" && i+1 < len(args) {
			outDir = args[i+1]
		}
	}
	if outDir == "" {
		return CommandResult{ExitCode: -1}, nil
	}
	outDir = strings.TrimRight(outDir, string(filepath.Separator))
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "results_sarif.sarif"), f.sarif, 0600); err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	return CommandResult{ExitCode: f.scanExitCode}, nil
}

func prepareCheckovFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "checkov.yml"), []byte("framework:\n  - terraform\nquiet: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "security_group.tf"), []byte("resource \"aws_security_group\" \"x\" {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCheckovScannerRunsPinnedCLIAndNormalizesEvidence(t *testing.T) {
	root := prepareCheckovFixture(t)
	runner := &checkovFakeRunner{sarif: []byte(checkovFixtureSARIF), scanExitCode: 1}
	scanner := CheckovScanner{
		Binary: "/opt/checkov", Version: "3.3.30", ConfigPath: "rules/checkov.yml",
		Timeout: time.Second, Runner: runner,
	}
	got, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if got.Status != ScanStatusComplete || len(got.Findings) != 1 {
		t.Fatalf("unexpected scan result: %+v", got)
	}
	if got.Findings[0].RuleID != "CKV_AWS_24" || got.Findings[0].Analyzer != "checkov" {
		t.Fatalf("finding identity lost: %+v", got.Findings[0])
	}
	if len(got.ConfigurationSHA256) != 64 || len(got.InputSHA256) != 64 || len(got.RawOutputSHA256) != 64 {
		t.Fatalf("missing evidence digests: %+v", got)
	}
	hasTF := false
	for _, p := range got.ScannedPaths {
		if p == "security_group.tf" {
			hasTF = true
		}
	}
	if !hasTF {
		t.Fatalf("expected terraform path in inventory: %+v", got.ScannedPaths)
	}
}

func TestCheckovScannerRejectsBinaryDigestMismatchBeforeExecution(t *testing.T) {
	root := prepareCheckovFixture(t)
	binary := filepath.Join(t.TempDir(), "fake-checkov")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runner := &checkovFakeRunner{sarif: []byte(checkovFixtureSARIF)}
	_, err := (CheckovScanner{
		Binary: binary, BinarySHA256: strings.Repeat("f", 64),
		Version: "3.3.30", ConfigPath: "rules/checkov.yml", Runner: runner,
	}).Scan(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "binary SHA-256 mismatch") {
		t.Fatalf("expected binary integrity failure, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unverified binary was executed: %+v", runner.calls)
	}
}

func TestCheckovScannerDropsFindingsOutsideEligibleInventory(t *testing.T) {
	root := prepareCheckovFixture(t)
	fixtureDir := filepath.Join(root, filepath.FromSlash(analyzerFixtureDirectory), "clean", "iac")
	if err := os.MkdirAll(fixtureDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "security_group.tf"), []byte("resource \"aws_security_group\" \"bad\" {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sarif := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov","version":"3.3.30","rules":[{"id":"CKV_AWS_24"}]}},"results":[{"ruleId":"CKV_AWS_24","level":"error","message":{"text":"open sg"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"testdata/analyzer-fixtures/clean/iac/security_group.tf"},"region":{"startLine":1}}}]},{"ruleId":"CKV_AWS_24","level":"error","message":{"text":"in-scope"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"security_group.tf"},"region":{"startLine":1}}}]}]}]}`
	runner := &checkovFakeRunner{sarif: []byte(sarif), scanExitCode: 1}
	got, err := (CheckovScanner{
		Binary: "/opt/checkov", Version: "3.3.30", ConfigPath: "rules/checkov.yml",
		Timeout: time.Second, Runner: runner, ScanPaths: []string{"security_group.tf"},
	}).Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(got.Findings) != 1 || got.Findings[0].Path != "security_group.tf" {
		t.Fatalf("expected only in-inventory finding, got %+v", got.Findings)
	}
	joined := strings.Join(runner.calls[len(runner.calls)-1].args, " ")
	if !strings.Contains(joined, "--skip-path "+analyzerFixtureDirectory) {
		t.Fatalf("expected Checkov skip-path for fixtures, args=%v", runner.calls[len(runner.calls)-1].args)
	}
}

func TestNormalizeCheckovRuleID(t *testing.T) {
	cases := map[string]string{
		"CKV_AWS_24":                     "CKV_AWS_24",
		"checkov.CKV_AWS_24":             "CKV_AWS_24",
		"https://docs/CKV_AWS_24":        "CKV_AWS_24",
		"bridgecrew.checkov.CKV2_AWS_11": "CKV2_AWS_11",
	}
	for in, want := range cases {
		if got := normalizeCheckovRuleID(in); got != want {
			t.Fatalf("normalizeCheckovRuleID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestVerifyScanEvidenceAcceptsCheckov(t *testing.T) {
	root := prepareCheckovFixture(t)
	raw := []byte(checkovFixtureSARIF)
	parsed, err := ParseSARIF(raw, root)
	if err != nil {
		t.Fatal(err)
	}
	configHash, err := HashFile(filepath.Join(root, "rules", "checkov.yml"))
	if err != nil {
		t.Fatal(err)
	}
	inputHash, err := HashInputTree(root)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Analyzers[0].Name = "checkov"
	parsed.Analyzers[0].Version = "3.3.30"
	parsed.Findings[0].Analyzer = "checkov"
	parsed.Findings[0].AnalyzerVersion = "3.3.30"
	parsed.Findings[0].RuleID = normalizeCheckovRuleID(parsed.Findings[0].RuleID)
	parsed.ConfigurationPath = "rules/checkov.yml"
	parsed.ConfigurationSHA256 = configHash
	parsed.BinarySHA256 = strings.Repeat("c", 64)
	parsed.RuleSetSHA256 = configHash
	parsed.InputSHA256 = inputHash
	parsed.ScannedPaths, err = discoverScannedPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyScanEvidence(root, parsed, raw)
	if err != nil {
		t.Fatalf("VerifyScanEvidence() error = %v", err)
	}
	if got.Findings[0].RuleID != "CKV_AWS_24" || got.Analyzers[0].Name != "checkov" {
		t.Fatalf("checkov verification lost identity: %+v", got)
	}
}
