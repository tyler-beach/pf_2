package analyzer

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

// VerifyScanEvidence reconstructs normalized findings from raw SARIF and checks
// that the scan is still bound to the current source tree and rule configuration.
// It deliberately does not trust the finding array in the normalized JSON file:
// findings are re-parsed from raw SARIF so hand-edited summaries cannot erase or
// invent violations. Digests provide integrity checks, not a digital signature.
func VerifyScanEvidence(root string, envelope ScanResult, rawSARIF []byte) (ScanResult, error) {
	fail := func(message string) (ScanResult, error) {
		envelope.Status = ScanStatusError
		envelope.Errors = append(envelope.Errors, message)
		envelope.Findings = nil
		envelope.ScannedPaths = nil
		return envelope, fmt.Errorf("%s", message)
	}
	if len(rawSARIF) == 0 {
		return fail("raw SARIF evidence is empty")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fail(fmt.Sprintf("resolve repository root: %v", err))
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fail(fmt.Sprintf("resolve repository root symlinks: %v", err))
	}
	if envelope.Status != ScanStatusComplete && envelope.Status != ScanStatusPartial {
		return fail(fmt.Sprintf("scan evidence status %q is not verifiable", envelope.Status))
	}
	if envelope.RawOutputSHA256 == "" || envelope.RawOutputSHA256 != sha256Hex(rawSARIF) {
		return fail("raw SARIF SHA-256 does not match normalized evidence metadata")
	}
	parsed, err := ParseSARIF(rawSARIF, rootAbs)
	if err != nil {
		return fail(fmt.Sprintf("parse raw SARIF evidence: %v", err))
	}
	if parsed.Status != envelope.Status {
		return fail(fmt.Sprintf("scan status mismatch: normalized metadata says %s but raw SARIF says %s", envelope.Status, parsed.Status))
	}
	if len(parsed.Analyzers) == 0 || len(envelope.Analyzers) != len(parsed.Analyzers) {
		return fail("analyzer metadata does not match raw SARIF runs")
	}
	for i := range parsed.Analyzers {
		if !strings.EqualFold(parsed.Analyzers[i].Name, envelope.Analyzers[i].Name) {
			return fail(fmt.Sprintf("analyzer metadata mismatch: raw SARIF reports %q but normalized metadata reports %q", parsed.Analyzers[i].Name, envelope.Analyzers[i].Name))
		}
		if !strings.Contains(strings.ToLower(parsed.Analyzers[i].Name), "opengrep") {
			return fail(fmt.Sprintf("unsupported analyzer in OpenGrep evidence: %q", parsed.Analyzers[i].Name))
		}
		if strings.TrimSpace(envelope.Analyzers[i].Version) == "" {
			return fail(fmt.Sprintf("verified version is missing for analyzer %q", parsed.Analyzers[i].Name))
		}
		parsed.Analyzers[i].Version = envelope.Analyzers[i].Version
		for fi := range parsed.Findings {
			if strings.EqualFold(parsed.Findings[fi].Analyzer, parsed.Analyzers[i].Name) {
				parsed.Findings[fi].AnalyzerVersion = envelope.Analyzers[i].Version
			}
		}
	}
	if strings.TrimSpace(envelope.ConfigurationPath) == "" {
		return fail("scan evidence is missing configuration_path")
	}
	configAbs, _, err := resolveConfig(rootAbs, envelope.ConfigurationPath)
	if err != nil {
		return fail(fmt.Sprintf("resolve recorded analyzer configuration: %v", err))
	}
	configHash, err := HashFile(configAbs)
	if err != nil {
		return fail(err.Error())
	}
	if envelope.ConfigurationSHA256 == "" || configHash != envelope.ConfigurationSHA256 {
		return fail("configuration SHA-256 does not match the current configuration file")
	}
	inputHash, err := HashInputTree(rootAbs)
	if err != nil {
		return fail(err.Error())
	}
	if envelope.InputSHA256 == "" || inputHash != envelope.InputSHA256 {
		return fail("input tree SHA-256 does not match the current repository snapshot")
	}
	paths, err := discoverScannedPathsExcluding(rootAbs, envelope.ConfigurationPath)
	if err != nil {
		return fail(fmt.Sprintf("rebuild repository scan inventory: %v", err))
	}
	parsed.ConfigurationPath = envelope.ConfigurationPath
	parsed.ConfigurationSHA256 = configHash
	parsed.InputSHA256 = inputHash
	if envelope.BinarySHA256 != "" && !isSHA256Digest(envelope.BinarySHA256) {
		return fail("binary SHA-256 provenance is malformed")
	}
	if envelope.RuleSetSHA256 != "" && envelope.RuleSetSHA256 != configHash {
		return fail("ruleset SHA-256 does not match the configuration digest")
	}
	parsed.BinarySHA256 = envelope.BinarySHA256
	parsed.RuleSetSHA256 = configHash
	parsed.ScannedPaths = paths
	inventory := make(map[string]bool, len(paths))
	for _, p := range paths { inventory[filepath.ToSlash(p)] = true }
	for _, finding := range parsed.Findings {
		if finding.Path != "" && !inventory[filepath.ToSlash(finding.Path)] {
			return fail(fmt.Sprintf("SARIF finding path %q is absent from the verified source inventory", finding.Path))
		}
	}
	parsed.RawSARIF = append([]byte(nil), rawSARIF...)
	parsed.Errors = append([]string(nil), envelope.Errors...)
	// Defensive check that the bytes used to construct the parsed result were
	// not mutated by a caller holding the original slice concurrently.
	if !bytes.Equal(parsed.RawSARIF, rawSARIF) {
		return fail("raw SARIF changed while verifying analyzer evidence")
	}
	return parsed, nil
}


func isSHA256Digest(value string) bool {
	if len(value) != 64 { return false }
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) { return false }
	}
	return true
}
