package rulecompile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VerifyResult is the outcome of verifying a published package.
type VerifyResult struct {
	OK           bool     `json:"ok"`
	PackageDir   string   `json:"package_dir"`
	RulesetSHA256 string  `json:"ruleset_sha256"`
	RuleCount    int      `json:"rule_count"`
	Errors       []string `json:"errors,omitempty"`
}

// VerifyPackage checks manifest digests and SHA256SUMS against package files.
func VerifyPackage(pkgDir string) (VerifyResult, error) {
	out := VerifyResult{PackageDir: pkgDir}
	manifestPath := filepath.Join(pkgDir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return out, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return out, err
	}
	rulesPath := filepath.Join(pkgDir, m.RulesFile)
	if m.RulesFile == "" {
		rulesPath = filepath.Join(pkgDir, "rules.yml")
	}
	rulesRaw, err := os.ReadFile(rulesPath)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out, nil
	}
	got := sha256Hex(rulesRaw)
	out.RulesetSHA256 = got
	out.RuleCount = m.RuleCount
	if got != m.RulesetSHA256 {
		out.Errors = append(out.Errors, fmt.Sprintf("ruleset_sha256 mismatch: manifest=%s file=%s", m.RulesetSHA256, got))
	}
	if m.RuleCount != len(m.RuleIDs) {
		out.Errors = append(out.Errors, fmt.Sprintf("rule_count %d != len(rule_ids) %d", m.RuleCount, len(m.RuleIDs)))
	}
	// Count - id: lines in rules.yml
	ids := ruleIDPattern.FindAllStringSubmatch(string(rulesRaw), -1)
	if len(ids) != m.RuleCount {
		out.Errors = append(out.Errors, fmt.Sprintf("rules.yml has %d ids, manifest rule_count=%d", len(ids), m.RuleCount))
	}
	for i, id := range m.RuleIDs {
		if i < len(ids) && ids[i][1] != id {
			// order should match sorted publish order
			out.Errors = append(out.Errors, fmt.Sprintf("rule id order mismatch at %d: manifest=%s file=%s", i, id, ids[i][1]))
			break
		}
	}
	sumsPath := filepath.Join(pkgDir, "SHA256SUMS")
	if sums, err := os.ReadFile(sumsPath); err == nil {
		for _, line := range strings.Split(string(sums), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) != 2 {
				continue
			}
			want, file := parts[0], parts[1]
			gotSum := sha256File(filepath.Join(pkgDir, file))
			if gotSum != want {
				out.Errors = append(out.Errors, fmt.Sprintf("SHA256SUMS mismatch for %s", file))
			}
		}
	}
	out.OK = len(out.Errors) == 0
	return out, nil
}
