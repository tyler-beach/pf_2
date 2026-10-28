package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func verificationFixture(t *testing.T) (string, []byte, ScanResult) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(root, "src"), 0700); err != nil { t.Fatal(err) }
	configPath := filepath.Join(root, "rules", "security.yml")
	if err := os.WriteFile(configPath, []byte("rules: []\n"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "src", "main.py"), []byte("print('safe')\n"), 0600); err != nil { t.Fatal(err) }
	raw := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Opengrep OSS Version","rules":[{"id":"RULE-1"}]}},"results":[{"ruleId":"RULE-1","level":"warning","message":{"text":"fixture finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/main.py"},"region":{"startLine":2,"startColumn":1}}}]}]}]}`)
	parsed, err := ParseSARIF(raw, root)
	if err != nil { t.Fatal(err) }
	configHash, err := HashFile(configPath)
	if err != nil { t.Fatal(err) }
	inputHash, err := HashInputTree(root)
	if err != nil { t.Fatal(err) }
	parsed.Analyzers[0].Version = "1.33.0"
	parsed.Findings[0].AnalyzerVersion = "1.33.0"
	parsed.ConfigurationPath = "rules/security.yml"
	parsed.ConfigurationSHA256 = configHash
	parsed.BinarySHA256 = strings.Repeat("a", 64)
	parsed.RuleSetSHA256 = strings.Repeat("b", 64)
	parsed.InputSHA256 = inputHash
	parsed.ScannedPaths, err = discoverScannedPaths(root)
	if err != nil { t.Fatal(err) }
	return root, raw, parsed
}

func TestVerifyScanEvidenceRebuildsFindingsFromRawSARIF(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	envelope.Findings[0].RuleID = "edited-rule-id"
	envelope.Findings[0].Message = "edited summary that hides real finding"
	got, err := VerifyScanEvidence(root, envelope, raw)
	if err != nil { t.Fatalf("VerifyScanEvidence() error = %v", err) }
	if got.Findings[0].RuleID != "RULE-1" || got.Findings[0].Message != "fixture finding" {
		t.Fatalf("summary JSON was trusted instead of raw SARIF: %+v", got.Findings[0])
	}
	if got.Status != ScanStatusComplete {
		t.Fatalf("verified scan is not complete: %+v", got)
	}
	hasMain := false
	for _, path := range got.ScannedPaths { if path == "src/main.py" { hasMain = true } }
	if !hasMain { t.Fatalf("verified inventory lost source file: %+v", got.ScannedPaths) }
}

func TestVerifyScanEvidenceRejectsRawOutputTampering(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	tampered := append([]byte(nil), raw...)
	tampered = append(tampered, []byte(" ")...)
	got, err := VerifyScanEvidence(root, envelope, tampered)
	if err == nil || got.Status != ScanStatusError || !strings.Contains(err.Error(), "raw SARIF SHA-256") {
		t.Fatalf("expected raw output integrity failure, got result=%+v err=%v", got, err)
	}
}

func TestVerifyScanEvidenceRejectsStaleSourceSnapshot(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	if err := os.WriteFile(filepath.Join(root, "src", "main.py"), []byte("print('changed')\n"), 0600); err != nil { t.Fatal(err) }
	got, err := VerifyScanEvidence(root, envelope, raw)
	if err == nil || got.Status != ScanStatusError || !strings.Contains(err.Error(), "input tree SHA-256") {
		t.Fatalf("expected stale input rejection, got result=%+v err=%v", got, err)
	}
}

func TestVerifyScanEvidenceRejectsRuleConfigurationDrift(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	if err := os.WriteFile(filepath.Join(root, "rules", "security.yml"), []byte("rules: [changed]\n"), 0600); err != nil { t.Fatal(err) }
	got, err := VerifyScanEvidence(root, envelope, raw)
	if err == nil || got.Status != ScanStatusError || !strings.Contains(err.Error(), "configuration SHA-256") {
		t.Fatalf("expected configuration drift rejection, got result=%+v err=%v", got, err)
	}
}

func TestVerifyScanEvidenceRejectsUnsupportedAnalyzerMetadata(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	envelope.Analyzers[0].Name = "another-tool"
	got, err := VerifyScanEvidence(root, envelope, raw)
	if err == nil || got.Status != ScanStatusError { t.Fatalf("expected analyzer metadata rejection, got %+v / %v", got, err) }
}

func TestVerifyScanEvidenceDoesNotTrustMissingVersionMetadata(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	envelope.Analyzers[0].Version = ""
	got, err := VerifyScanEvidence(root, envelope, raw)
	if err == nil || got.Status != ScanStatusError { t.Fatalf("expected missing-version rejection, got %+v / %v", got, err) }
}

// Verify normalized JSON schema remains independently consumable by automation.
func TestVerificationFixtureMetadataIsJSONSerializable(t *testing.T) {
	_, _, envelope := verificationFixture(t)
	if _, err := json.Marshal(envelope); err != nil { t.Fatalf("marshal metadata: %v", err) }
}


func TestVerifyScanEvidenceAcceptsRelativeRepositoryRoot(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	previous, err := os.Getwd()
	if err != nil { t.Fatal(err) }
	if err := os.Chdir(root); err != nil { t.Fatal(err) }
	defer func() {
		if err := os.Chdir(previous); err != nil { t.Errorf("restore working directory: %v", err) }
	}()
	got, err := VerifyScanEvidence(".", envelope, raw)
	if err != nil { t.Fatalf("relative root should resolve to an absolute repository path: %v", err) }
	if got.Status != ScanStatusComplete { t.Fatalf("status = %q, want COMPLETE", got.Status) }
}


func TestVerifyScanEvidenceRejectsFindingOutsideVerifiedInventory(t *testing.T) {
	root, _, envelope := verificationFixture(t)
	raw := []byte("{\"version\":\"2.1.0\",\"runs\":[{\"tool\":{\"driver\":{\"name\":\"Opengrep OSS Version\",\"rules\":[{\"id\":\"RULE-1\"}]}},\"results\":[{\"ruleId\":\"RULE-1\",\"message\":{\"text\":\"finding\"},\"locations\":[{\"physicalLocation\":{\"artifactLocation\":{\"uri\":\"src/not-present.py\"},\"region\":{\"startLine\":1}}}]}]}]}")
	envelope.RawOutputSHA256 = sha256Hex(raw)
	got, err := VerifyScanEvidence(root, envelope, raw)
	if err == nil || got.Status != ScanStatusError || !strings.Contains(err.Error(), "absent from the verified source inventory") {
		t.Fatalf("expected finding/inventory mismatch rejection, got result=%+v err=%v", got, err)
	}
}


func TestVerifyScanEvidenceRejectsMissingBinaryOrRuleSetDigest(t *testing.T) {
	tests := []struct { name string; clear func(*ScanResult) }{
		{"binary digest", func(e *ScanResult) { e.BinarySHA256 = "" }},
		{"ruleset digest", func(e *ScanResult) { e.RuleSetSHA256 = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, raw, envelope := verificationFixture(t)
			tc.clear(&envelope)
			got, err := VerifyScanEvidence(root, envelope, raw)
			if err == nil || got.Status != ScanStatusError {
				t.Fatalf("missing %s must reject evidence: result=%+v err=%v", tc.name, got, err)
			}
		})
	}
}

func TestVerifyScopedScanEvidenceRequiresExactPlannedInventory(t *testing.T) {
	root, raw, envelope := verificationFixture(t)
	envelope.ScannedPaths = []string{"src/main.py"}
	got, err := VerifyScopedScanEvidence(root, envelope, raw, []string{"src/main.py"})
	if err != nil { t.Fatalf("valid scoped evidence rejected: %v", err) }
	if len(got.ScannedPaths) != 1 || got.ScannedPaths[0] != "src/main.py" {
		t.Fatalf("verified scope mismatch: %v", got.ScannedPaths)
	}

	envelope.ScannedPaths = []string{"src/other.py"}
	got, err = VerifyScopedScanEvidence(root, envelope, raw, []string{"src/main.py"})
	if err == nil || got.Status != ScanStatusError || !strings.Contains(err.Error(), "planned scope") {
		t.Fatalf("expected scope mismatch rejection, got result=%+v err=%v", got, err)
	}
}
