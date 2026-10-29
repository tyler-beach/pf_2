package rulecompile

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const ManifestSchemaVersion = "1"

var semverPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+([.-][0-9A-Za-z.-]+)?$`)

// Manifest describes a published, pin-able OpenGrep ruleset for Pathfinder.
type Manifest struct {
	SchemaVersion    string            `json:"schema_version"`
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	CreatedAt        string            `json:"created_at"`
	RulesetSHA256    string            `json:"ruleset_sha256"`
	RulesFile        string            `json:"rules_file"`
	BindingsFile     string            `json:"bindings_file"`
	RuleCount        int               `json:"rule_count"`
	BindingCount     int               `json:"binding_count"`
	RuleIDs          []string          `json:"rule_ids"`
	SourceRoot       string            `json:"source_root,omitempty"`
	SourceRootSHA256 string            `json:"source_root_sha256,omitempty"`
	PromptVersion    string            `json:"prompt_version"`
	Provider         string            `json:"provider"`
	Model            string            `json:"model"`
	OpenGrepVersion  string            `json:"opengrep_version,omitempty"`
	MinGateStatus    string            `json:"min_gate_status"` // published rules required "passed"
	Notes            []string          `json:"notes"`
	PathfinderHint   map[string]string `json:"pathfinder_consume,omitempty"`
}

// Binding is a suggested obligation→rule link for human Pathfinder review.
// It is not an approved requirements.json entry.
type Binding struct {
	CandidateID         string  `json:"candidate_id"`
	ADRID               string  `json:"adr_id"`
	Statement           string  `json:"statement"`
	Modality            string  `json:"modality"`
	Route               Route   `json:"route"`
	RuleID              string  `json:"rule_id,omitempty"`
	ExistingRuleIDs     []string `json:"existing_rule_ids,omitempty"`
	PatternID           string  `json:"pattern_id,omitempty"`
	CombinedConfidenceP float64 `json:"combined_confidence_p"`
	CombinedConfidence  string  `json:"combined_confidence"`
	GateStatus          string  `json:"gate_status"`
	SourcePath          string  `json:"source_path"`
	SourceLineStart     int     `json:"source_line_start"`
	SourceLineEnd       int     `json:"source_line_end"`
	Executable          bool    `json:"executable"` // always false
}

// PublishOptions configures ruleset packaging.
type PublishOptions struct {
	Name       string // e.g. pathfinder-adr-corpus
	Version    string // semver, with or without leading v
	FromDir    string // compile out dir containing report.json
	OutDir     string // parent directory for {name}-{version}/
	TarGz      bool   // also write {name}-{version}.tar.gz
	IncludeDrafts bool // copy provenance/drafts (larger)
	Now        func() time.Time
}

// PublishResult points at the written package.
type PublishResult struct {
	PackageDir    string
	ManifestPath  string
	RulesPath     string
	BindingsPath  string
	TarGzPath     string
	Manifest      Manifest
}

// Publish builds a versioned ruleset package from a compile report directory.
// Only gate-passed compile_new rules enter rules.yml. Bindings include all routes.
func Publish(opts PublishOptions) (PublishResult, error) {
	var zero PublishResult
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "pathfinder-adr-rules"
	}
	version := strings.TrimSpace(opts.Version)
	if !semverPattern.MatchString(version) {
		return zero, fmt.Errorf("--version must be semver (got %q)", version)
	}
	version = strings.TrimPrefix(version, "v")
	if strings.TrimSpace(opts.FromDir) == "" {
		return zero, fmt.Errorf("--from is required (compile output directory)")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return zero, fmt.Errorf("--out is required")
	}
	fromAbs, err := filepath.Abs(opts.FromDir)
	if err != nil {
		return zero, err
	}
	outParent, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return zero, err
	}
	reportPath := filepath.Join(fromAbs, "report.json")
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return zero, fmt.Errorf("read compile report: %w", err)
	}
	var report Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return zero, fmt.Errorf("decode report.json: %w", err)
	}

	pkgName := fmt.Sprintf("%s-%s", name, version)
	pkgDir := filepath.Join(outParent, pkgName)
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		return zero, err
	}
	provDir := filepath.Join(pkgDir, "provenance")
	if err := os.MkdirAll(provDir, 0o750); err != nil {
		return zero, err
	}
	if err := copyFile(reportPath, filepath.Join(provDir, "report.json")); err != nil {
		return zero, err
	}
	candSrc := filepath.Join(fromAbs, "candidates.json")
	if _, err := os.Stat(candSrc); err == nil {
		_ = copyFile(candSrc, filepath.Join(provDir, "candidates.json"))
	}
	if opts.IncludeDrafts {
		srcDrafts := filepath.Join(fromAbs, "drafts")
		if st, err := os.Stat(srcDrafts); err == nil && st.IsDir() {
			if err := copyTree(srcDrafts, filepath.Join(provDir, "drafts")); err != nil {
				return zero, err
			}
		}
	}

	rulesYAML, ruleIDs, bindings, openGrepVer, err := assemblePublishedArtifacts(report)
	if err != nil {
		return zero, err
	}
	rulesPath := filepath.Join(pkgDir, "rules.yml")
	if err := os.WriteFile(rulesPath, []byte(rulesYAML), 0o600); err != nil {
		return zero, err
	}
	rulesetSHA := sha256Hex([]byte(rulesYAML))
	bindingsPath := filepath.Join(pkgDir, "bindings.json")
	if err := writeJSON(bindingsPath, map[string]any{
		"schema_version": ManifestSchemaVersion,
		"executable":     false,
		"note":           "Suggested obligation→rule links only. Not Pathfinder requirements.json.",
		"bindings":       bindings,
	}); err != nil {
		return zero, err
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	manifest := Manifest{
		SchemaVersion:   ManifestSchemaVersion,
		Name:            name,
		Version:         version,
		CreatedAt:       nowFn().UTC().Format(time.RFC3339),
		RulesetSHA256:   rulesetSHA,
		RulesFile:       "rules.yml",
		BindingsFile:    "bindings.json",
		RuleCount:       len(ruleIDs),
		BindingCount:    len(bindings),
		RuleIDs:         ruleIDs,
		SourceRoot:      report.SourceRoot,
		PromptVersion:   report.PromptVersion,
		Provider:        report.Provider,
		Model:           report.Model,
		OpenGrepVersion: openGrepVer,
		MinGateStatus:   "passed",
		Notes: []string{
			"Published rules are OpenGrep/Semgrep YAML for Pathfinder analyzer-rule scans.",
			"Pin ruleset_sha256 in plan/scan manifests. Do not treat bindings as approved policy.",
			"Compile-time LLM only; Pathfinder assess remains zero-LLM.",
		},
		PathfinderHint: map[string]string{
			"scan_config":    "rules.yml",
			"ruleset_sha256": rulesetSHA,
			"example":        "adrctl scan-opengrep --config <pkg>/rules.yml --binary <opengrep> ...",
		},
	}
	if sum, err := hashTree(report.SourceRoot); err == nil {
		manifest.SourceRootSHA256 = sum
	}
	manifestPath := filepath.Join(pkgDir, "manifest.json")
	if err := writeJSON(manifestPath, manifest); err != nil {
		return zero, err
	}
	sums := fmt.Sprintf("%s  rules.yml\n%s  bindings.json\n%s  manifest.json\n",
		rulesetSHA, sha256File(bindingsPath), sha256File(manifestPath))
	if err := os.WriteFile(filepath.Join(pkgDir, "SHA256SUMS"), []byte(sums), 0o600); err != nil {
		return zero, err
	}

	result := PublishResult{
		PackageDir:   pkgDir,
		ManifestPath: manifestPath,
		RulesPath:    rulesPath,
		BindingsPath: bindingsPath,
		Manifest:     manifest,
	}
	if opts.TarGz {
		tarPath := pkgDir + ".tar.gz"
		if err := writeTarGz(tarPath, pkgDir, pkgName); err != nil {
			return zero, err
		}
		result.TarGzPath = tarPath
	}
	return result, nil
}

func assemblePublishedArtifacts(report Report) (rulesYAML string, ruleIDs []string, bindings []Binding, openGrepVer string, err error) {
	type publishedRule struct {
		id   string
		body string // indented rule block without leading "- id"
		yaml string // full single-rule document fragment under rules:
	}
	var published []publishedRule
	seen := map[string]string{} // rule id -> candidate

	for _, it := range report.Items {
		b := Binding{
			CandidateID:         it.CandidateID,
			ADRID:               it.ADRID,
			Statement:           it.Statement,
			Modality:            it.Modality,
			Route:               it.Draft.Route,
			PatternID:           it.Draft.PatternID,
			ExistingRuleIDs:     append([]string(nil), it.Draft.ExistingRuleIDs...),
			CombinedConfidenceP: it.CombinedConfidenceP,
			CombinedConfidence:  it.CombinedConfidence,
			GateStatus:          it.Gate.Status,
			SourcePath:          it.SourcePath,
			SourceLineStart:     it.SourceLineStart,
			SourceLineEnd:       it.SourceLineEnd,
			Executable:          false,
		}
		if openGrepVer == "" && it.Gate.OpenGrepVersion != "" {
			openGrepVer = it.Gate.OpenGrepVersion
		}

		if it.Draft.Route == RouteCompileNew && it.Gate.Status == "passed" && strings.TrimSpace(it.Draft.RuleYAML) != "" {
			stableID := stableRuleID(it)
			fragment, err := rewriteRuleYAML(it.Draft.RuleYAML, stableID)
			if err != nil {
				return "", nil, nil, "", fmt.Errorf("%s: %w", it.CandidateID, err)
			}
			if prev, ok := seen[stableID]; ok {
				return "", nil, nil, "", fmt.Errorf("duplicate rule id %q from %s and %s", stableID, prev, it.CandidateID)
			}
			seen[stableID] = it.CandidateID
			published = append(published, publishedRule{id: stableID, yaml: fragment})
			b.RuleID = stableID
		} else if it.Draft.Route == RouteBindExisting && len(it.Draft.ExistingRuleIDs) > 0 {
			b.RuleID = it.Draft.ExistingRuleIDs[0]
		}
		bindings = append(bindings, b)
	}

	sort.SliceStable(published, func(i, j int) bool { return published[i].id < published[j].id })
	var b strings.Builder
	b.WriteString("# Generated by adrcompile — do not hand-edit without re-publish\n")
	b.WriteString("# Gate: OpenGrep bad>=1 / good==0. Not Pathfinder-approved until bound.\n")
	b.WriteString("rules:\n")
	for _, pr := range published {
		ruleIDs = append(ruleIDs, pr.id)
		// fragment is a full "rules:\n  - id: ..." doc; extract body after rules:
		body := extractRulesBody(pr.yaml)
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String(), ruleIDs, bindings, openGrepVer, nil
}

func stableRuleID(it Item) string {
	adr := strings.ToLower(strings.ReplaceAll(it.ADRID, " ", "-"))
	adr = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, adr)
	short := it.CandidateID
	if len(short) > 12 {
		// CAND-<hex> → keep trailing hex
		if i := strings.LastIndex(short, "-"); i >= 0 && i+1 < len(short) {
			short = short[i+1:]
		}
		if len(short) > 12 {
			short = short[:12]
		}
	}
	short = strings.ToLower(short)
	return fmt.Sprintf("pathfinder.compiled.%s.%s", adr, short)
}

func rewriteRuleYAML(yamlText, newID string) (string, error) {
	yamlText = strings.TrimSpace(yamlText) + "\n"
	if !strings.Contains(yamlText, "rules:") {
		return "", fmt.Errorf("rule yaml missing rules:")
	}
	replaced := false
	lines := strings.Split(yamlText, "\n")
	for i, line := range lines {
		if m := ruleIDPattern.FindStringSubmatch(line); len(m) == 2 {
			indent := line[:strings.Index(line, "-")]
			lines[i] = indent + "- id: " + newID
			replaced = true
			break
		}
	}
	if !replaced {
		return "", fmt.Errorf("could not find id: line to rewrite")
	}
	return strings.Join(lines, "\n"), nil
}

func extractRulesBody(yamlText string) string {
	yamlText = strings.TrimSpace(yamlText) + "\n"
	idx := strings.Index(yamlText, "rules:")
	if idx < 0 {
		return yamlText
	}
	body := yamlText[idx+len("rules:"):]
	body = strings.TrimLeft(body, "\n")
	// ensure each top-level rule line keeps two-space indent under rules:
	return body
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sha256File(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return sha256Hex(raw)
}

func hashTree(root string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\n%x\n", filepath.ToSlash(rel), sha256.Sum256(raw))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o600)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func writeTarGz(tarPath, dir, rootName string) error {
	f, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(rootName, rel))
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
}
