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
	return verifyScanEvidence(root, envelope, rawSARIF, nil)
}

// VerifyScopedScanEvidence verifies a result against the exact source inventory
// declared by a previously verified execution plan. It prevents a runner from
// claiming a narrow scope while actually scanning a different file set.
func VerifyScopedScanEvidence(root string, envelope ScanResult, rawSARIF []byte, expectedPaths []string) (ScanResult, error) {
	if len(expectedPaths) == 0 {
		envelope.Status = ScanStatusError
		envelope.Errors = append(envelope.Errors, "planned scan scope is empty")
		envelope.Findings = nil
		envelope.ScannedPaths = nil
		return envelope, fmt.Errorf("planned scan scope is empty")
	}
	return verifyScanEvidence(root, envelope, rawSARIF, expectedPaths)
}

func verifyScanEvidence(root string, envelope ScanResult, rawSARIF []byte, expectedPaths []string) (ScanResult, error) {
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
	if !validPlanSHA256(strings.ToLower(strings.TrimSpace(envelope.BinarySHA256))) {
		return fail("scan evidence is missing a valid binary_sha256")
	}
	if !validPlanSHA256(strings.ToLower(strings.TrimSpace(envelope.RuleSetSHA256))) {
		return fail("scan evidence is missing a valid rule_set_sha256")
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
		rawName := strings.ToLower(strings.TrimSpace(parsed.Analyzers[i].Name))
		envName := strings.ToLower(strings.TrimSpace(envelope.Analyzers[i].Name))
		if !analyzerNamesCompatible(rawName, envName) {
			return fail(fmt.Sprintf("analyzer metadata mismatch: raw SARIF reports %q but normalized metadata reports %q", parsed.Analyzers[i].Name, envelope.Analyzers[i].Name))
		}
		family := supportedAnalyzerFamily(rawName)
		if family == "" {
			family = supportedAnalyzerFamily(envName)
		}
		if family == "" {
			return fail(fmt.Sprintf("unsupported analyzer in verified evidence: %q", parsed.Analyzers[i].Name))
		}
		if strings.TrimSpace(envelope.Analyzers[i].Version) == "" {
			return fail(fmt.Sprintf("verified version is missing for analyzer %q", parsed.Analyzers[i].Name))
		}
		parsed.Analyzers[i].Name = family
		parsed.Analyzers[i].Version = envelope.Analyzers[i].Version
		for fi := range parsed.Findings {
			findingAnalyzer := strings.ToLower(strings.TrimSpace(parsed.Findings[fi].Analyzer))
			if analyzerNamesCompatible(findingAnalyzer, rawName) || analyzerNamesCompatible(findingAnalyzer, envName) || findingAnalyzer == "" {
				parsed.Findings[fi].Analyzer = family
				parsed.Findings[fi].AnalyzerVersion = envelope.Analyzers[i].Version
			}
			if family == "checkov" {
				parsed.Findings[fi].RuleID = normalizeCheckovRuleID(parsed.Findings[fi].RuleID)
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
	var paths []string
	if expectedPaths != nil {
		paths, err = normalizeScanPaths(rootAbs, expectedPaths, envelope.ConfigurationPath)
		if err != nil {
			return fail(fmt.Sprintf("validate planned scan inventory: %v", err))
		}
		if len(envelope.ScannedPaths) != len(paths) {
			return fail("scanned path inventory does not match planned scope")
		}
		for i := range paths {
			if filepath.ToSlash(envelope.ScannedPaths[i]) != paths[i] {
				return fail("scanned path inventory does not match planned scope")
			}
		}
	} else {
		paths, err = discoverScannedPathsExcluding(rootAbs, envelope.ConfigurationPath)
		if err != nil {
			return fail(fmt.Sprintf("rebuild repository scan inventory: %v", err))
		}
	}
	parsed.ConfigurationPath = envelope.ConfigurationPath
	parsed.ConfigurationSHA256 = configHash
	parsed.BinarySHA256 = strings.ToLower(strings.TrimSpace(envelope.BinarySHA256))
	parsed.RuleSetSHA256 = strings.ToLower(strings.TrimSpace(envelope.RuleSetSHA256))
	parsed.InputSHA256 = inputHash
	parsed.ScannedPaths = paths
	inventory := make(map[string]bool, len(paths))
	for _, p := range paths {
		inventory[filepath.ToSlash(p)] = true
	}
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

func supportedAnalyzerFamily(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "opengrep"):
		return "opengrep"
	case strings.Contains(name, "checkov"):
		return "checkov"
	default:
		return ""
	}
}

func analyzerNamesCompatible(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	fa, fb := supportedAnalyzerFamily(a), supportedAnalyzerFamily(b)
	return fa != "" && fa == fb
}
