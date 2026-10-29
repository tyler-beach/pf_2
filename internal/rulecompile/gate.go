package rulecompile

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var ruleIDPattern = regexp.MustCompile(`(?m)^\s*-\s*id:\s*([A-Za-z0-9._-]+)`)

// structuralValidate checks the draft has the minimum fields for an OpenGrep rule.
func structuralValidate(d Draft) GateResult {
	out := GateResult{Status: "failed", StructuralOK: false}
	if d.Route != RouteCompileNew {
		out.Status = "not_applicable"
		out.StructuralOK = true
		return out
	}
	if strings.TrimSpace(d.RuleYAML) == "" {
		out.Errors = append(out.Errors, "rule_yaml is empty")
		return out
	}
	if !strings.Contains(d.RuleYAML, "rules:") || !strings.Contains(d.RuleYAML, "id:") {
		out.Errors = append(out.Errors, "rule_yaml missing rules:/id:")
		return out
	}
	id := d.RuleID
	if id == "" {
		if m := ruleIDPattern.FindStringSubmatch(d.RuleYAML); len(m) == 2 {
			id = m[1]
		}
	}
	out.RuleID = id
	if id == "" {
		out.Errors = append(out.Errors, "could not determine rule id")
		return out
	}
	if strings.TrimSpace(d.BadContent) == "" || strings.TrimSpace(d.GoodContent) == "" {
		out.Errors = append(out.Errors, "bad_content and good_content are required for compile_new")
		return out
	}
	if strings.TrimSpace(d.BadFilename) == "" || strings.TrimSpace(d.GoodFilename) == "" {
		out.Errors = append(out.Errors, "bad_filename and good_filename are required")
		return out
	}
	out.StructuralOK = true
	out.Status = "passed"
	return out
}

// runOpenGrepGates writes fixtures and requires >=1 hit on bad and 0 on good.
func runOpenGrepGates(binary string, draftDir string, d Draft) GateResult {
	base := structuralValidate(d)
	if d.Route != RouteCompileNew {
		return base
	}
	if !base.StructuralOK {
		return base
	}
	base.Status = "failed"
	base.OpenGrepBinary = binary

	if strings.TrimSpace(binary) == "" {
		base.Status = "unavailable"
		base.Errors = append(base.Errors, "opengrep binary not provided; structural checks only")
		return base
	}
	if _, err := os.Stat(binary); err != nil {
		base.Status = "unavailable"
		base.Errors = append(base.Errors, fmt.Sprintf("opengrep binary not found: %v", err))
		return base
	}

	versionOut, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		base.Status = "unavailable"
		base.Errors = append(base.Errors, fmt.Sprintf("opengrep --version failed: %v (%s)", err, truncate(string(versionOut), 200)))
		return base
	}
	base.OpenGrepVersion = strings.TrimSpace(string(versionOut))

	rulePath := filepath.Join(draftDir, "rule.yml")
	if err := os.WriteFile(rulePath, []byte(d.RuleYAML), 0o600); err != nil {
		base.Errors = append(base.Errors, err.Error())
		return base
	}
	fixDir := filepath.Join(draftDir, "fixtures")
	if err := os.MkdirAll(fixDir, 0o750); err != nil {
		base.Errors = append(base.Errors, err.Error())
		return base
	}
	badPath := filepath.Join(fixDir, filepath.Base(d.BadFilename))
	goodPath := filepath.Join(fixDir, filepath.Base(d.GoodFilename))
	if err := os.WriteFile(badPath, []byte(d.BadContent), 0o600); err != nil {
		base.Errors = append(base.Errors, err.Error())
		return base
	}
	if err := os.WriteFile(goodPath, []byte(d.GoodContent), 0o600); err != nil {
		base.Errors = append(base.Errors, err.Error())
		return base
	}

	badCount, badErr := countOpenGrepFindings(binary, rulePath, badPath)
	goodCount, goodErr := countOpenGrepFindings(binary, rulePath, goodPath)
	base.BadFindings = badCount
	base.GoodFindings = goodCount
	if badErr != nil {
		base.Errors = append(base.Errors, "bad fixture: "+badErr.Error())
	}
	if goodErr != nil {
		base.Errors = append(base.Errors, "good fixture: "+goodErr.Error())
	}
	if badCount < 1 {
		base.Errors = append(base.Errors, fmt.Sprintf("expected >=1 finding on bad fixture, got %d", badCount))
	}
	if goodCount != 0 {
		base.Errors = append(base.Errors, fmt.Sprintf("expected 0 findings on good fixture, got %d", goodCount))
	}
	if len(base.Errors) == 0 {
		base.Status = "passed"
	}
	return base
}

func countOpenGrepFindings(binary, rulePath, target string) (int, error) {
	// Prefer JSON for stable counting; fall back to parsing text if needed.
	cmd := exec.Command(binary, "scan", "--json", "-f", rulePath, target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// OpenGrep/Semgrep exit 0 = clean, 1 = findings; both OK for counting.
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() != 0 && ee.ExitCode() != 1 {
				return 0, fmt.Errorf("exit %d: %s", ee.ExitCode(), truncate(stderr.String(), 400))
			}
		} else {
			return 0, err
		}
	}
	out := stdout.Bytes()
	if len(out) == 0 {
		return 0, nil
	}
	// Cheap count: "check_id" / "rule_id" occurrences in results arrays.
	// Prefer results length via a tiny JSON peek without importing encoding/json schemas.
	n := countJSONResults(out)
	return n, nil
}

func countJSONResults(raw []byte) int {
	// Match Semgrep/OpenGrep JSON "results": [ ... ]
	const key = `"results"`
	idx := bytes.Index(raw, []byte(key))
	if idx < 0 {
		return 0
	}
	rest := raw[idx+len(key):]
	bracket := bytes.IndexByte(rest, '[')
	if bracket < 0 {
		return 0
	}
	rest = rest[bracket+1:]
	depth := 1
	count := 0
	inString := false
	escape := false
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			if depth == 1 {
				count++
			}
			depth++
		case '}':
			depth--
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return count
			}
		}
	}
	return count
}
