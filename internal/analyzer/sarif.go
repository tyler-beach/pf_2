// Package analyzer normalizes third-party static-analysis output into auditable,
 // repository-relative evidence. It intentionally has no dependency on a particular
 // scanner so analyzers can be tested and upgraded independently.
package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	ScanStatusComplete = "COMPLETE"
	ScanStatusPartial  = "PARTIAL"
	ScanStatusError    = "ERROR"
	SARIFVersion       = "2.1.0"
)

type AnalyzerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type Finding struct {
	Analyzer        string         `json:"analyzer"`
	AnalyzerVersion string         `json:"analyzer_version,omitempty"`
	RuleID          string         `json:"rule_id"`
	Message         string         `json:"message"`
	Path            string         `json:"path,omitempty"`
	StartLine       int            `json:"start_line,omitempty"`
	StartColumn     int            `json:"start_column,omitempty"`
	Level           string         `json:"level"`
	Fingerprint     string         `json:"fingerprint"`
	Properties      map[string]any `json:"properties,omitempty"`
}

type ScanResult struct {
	SchemaVersion   string         `json:"schema_version"`
	SARIFVersion    string         `json:"sarif_version"`
	Status          string         `json:"status"`
	Analyzers       []AnalyzerInfo `json:"analyzers"`
	ConfigurationPath   string `json:"configuration_path,omitempty"`
	ConfigurationSHA256 string `json:"configuration_sha256,omitempty"`
	BinarySHA256        string `json:"binary_sha256,omitempty"`
	RuleSetSHA256       string `json:"rule_set_sha256,omitempty"`
	InputSHA256         string `json:"input_sha256,omitempty"`
	RawOutputSHA256 string         `json:"raw_output_sha256"`
	ScannedPaths    []string       `json:"scanned_paths"`
	RawSARIF        []byte         `json:"-"`
	Findings        []Finding      `json:"findings"`
	Errors          []string       `json:"errors,omitempty"`
}

type sarifDocument struct {
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool        `json:"tool"`
	Results     []sarifResult    `json:"results"`
	Invocations []sarifInvocation `json:"invocations"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name    string     `json:"name"`
	Version string     `json:"version"`
	Rules   []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID string `json:"id"`
}

type sarifInvocation struct {
	ExecutionSuccessful *bool                 `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications"`
}

type sarifNotification struct {
	Message sarifMessage `json:"message"`
}

type sarifResult struct {
	RuleID             string            `json:"ruleId"`
	RuleIndex          *int              `json:"ruleIndex"`
	Level              string            `json:"level"`
	Message            sarifMessage      `json:"message"`
	Locations          []sarifLocation   `json:"locations"`
	Fingerprints       map[string]string `json:"fingerprints"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties         map[string]any     `json:"properties"`
}

type sarifMessage struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

// ParseSARIF validates SARIF 2.1.0 and converts results into stable evidence.
// Any path included by the SARIF producer must resolve inside repoRoot; external
// paths fail closed rather than being silently attributed to this repository.
func ParseSARIF(data []byte, repoRoot string) (ScanResult, error) {
	out := ScanResult{
		SchemaVersion: "1",
		SARIFVersion: SARIFVersion,
		Status: ScanStatusComplete,
		Findings: []Finding{},
		Errors: []string{},
		Analyzers: []AnalyzerInfo{},
		RawOutputSHA256: sha256Hex(data),
		ScannedPaths: []string{},
		RawSARIF: append([]byte(nil), data...),
	}
	var doc sarifDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return out, fmt.Errorf("decode SARIF JSON: %w", err)
	}
	if doc.Version != SARIFVersion {
		return out, fmt.Errorf("unsupported SARIF version %q (expected %s)", doc.Version, SARIFVersion)
	}
	if len(doc.Runs) == 0 {
		return out, fmt.Errorf("SARIF document contains no runs")
	}
	if repoRoot == "" {
		repoRoot = "."
	}
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return out, fmt.Errorf("resolve repository root: %w", err)
	}

	seenAnalyzers := map[string]bool{}
	for runIndex, run := range doc.Runs {
		name := strings.TrimSpace(run.Tool.Driver.Name)
		if name == "" {
			return out, fmt.Errorf("SARIF run %d has no tool.driver.name", runIndex)
		}
		version := strings.TrimSpace(run.Tool.Driver.Version)
		key := name + "\x00" + version
		if !seenAnalyzers[key] {
			out.Analyzers = append(out.Analyzers, AnalyzerInfo{Name: name, Version: version})
			seenAnalyzers[key] = true
		}
		for _, invocation := range run.Invocations {
			if invocation.ExecutionSuccessful != nil && !*invocation.ExecutionSuccessful {
				out.Status = ScanStatusPartial
				out.Errors = append(out.Errors, fmt.Sprintf("%s reported an unsuccessful invocation", name))
			}
			for _, notification := range invocation.ToolExecutionNotifications {
				message := strings.TrimSpace(notification.Message.Text)
				if message == "" { message = strings.TrimSpace(notification.Message.Markdown) }
				if message != "" {
					out.Status = ScanStatusPartial
					out.Errors = append(out.Errors, name+": "+message)
				}
			}
		}

		for resultIndex, item := range run.Results {
			ruleID := strings.TrimSpace(item.RuleID)
			if ruleID == "" && item.RuleIndex != nil {
				if *item.RuleIndex < 0 || *item.RuleIndex >= len(run.Tool.Driver.Rules) {
					return out, fmt.Errorf("SARIF run %d result %d has out-of-range ruleIndex %d", runIndex, resultIndex, *item.RuleIndex)
				}
				ruleID = strings.TrimSpace(run.Tool.Driver.Rules[*item.RuleIndex].ID)
			}
			if ruleID == "" {
				return out, fmt.Errorf("SARIF run %d result %d has no ruleId or resolvable ruleIndex", runIndex, resultIndex)
			}
			message := strings.TrimSpace(item.Message.Text)
			if message == "" { message = strings.TrimSpace(item.Message.Markdown) }
			if message == "" {
				return out, fmt.Errorf("SARIF run %d result %d has no message", runIndex, resultIndex)
			}
			level := strings.ToLower(strings.TrimSpace(item.Level))
			switch level {
			case "error", "warning", "note", "none":
			case "":
				level = "warning"
			default:
				return out, fmt.Errorf("SARIF run %d result %d has invalid level %q", runIndex, resultIndex, level)
			}

			f := Finding{
				Analyzer: name,
				AnalyzerVersion: version,
				RuleID: ruleID,
				Message: message,
				Level: level,
				Properties: item.Properties,
			}
			if len(item.Locations) > 0 {
				physical := item.Locations[0].PhysicalLocation
				if strings.TrimSpace(physical.ArtifactLocation.URI) != "" {
					normalized, err := normalizeArtifactPath(physical.ArtifactLocation.URI, rootAbs)
					if err != nil {
						return out, fmt.Errorf("SARIF run %d result %d: %w", runIndex, resultIndex, err)
					}
					f.Path = normalized
				}
				if physical.Region.StartLine > 0 { f.StartLine = physical.Region.StartLine }
				if physical.Region.StartColumn > 0 { f.StartColumn = physical.Region.StartColumn }
			}
			f.Fingerprint = findingFingerprint(item, f)
			out.Findings = append(out.Findings, f)
		}
	}
	deduplicatedFindings, dedupErr := deduplicateAndSortFindings(out.Findings)
	if dedupErr != nil {
		return out, dedupErr
	}
	out.Findings = deduplicatedFindings

	sort.Slice(out.Analyzers, func(i, j int) bool {
		if out.Analyzers[i].Name == out.Analyzers[j].Name {
			return out.Analyzers[i].Version < out.Analyzers[j].Version
		}
		return out.Analyzers[i].Name < out.Analyzers[j].Name
	})
	sort.Strings(out.Errors)
	return out, nil
}

// deduplicateAndSortFindings removes byte-for-byte equivalent normalized finding
// records and applies a deterministic total order. It deliberately includes the
// complete normalized finding in the identity so a reused producer fingerprint
// cannot cause a distinct message or evidence-property set to be discarded.
func deduplicateAndSortFindings(findings []Finding) ([]Finding, error) {
	type keyedFinding struct {
		finding  Finding
		identity string
	}
	keyed := make([]keyedFinding, 0, len(findings))
	for _, finding := range findings {
		encoded, err := json.Marshal(finding)
		if err != nil {
			return nil, fmt.Errorf("encode normalized finding identity: %w", err)
		}
		keyed = append(keyed, keyedFinding{
			finding:  finding,
			identity: string(encoded),
		})
	}

	sort.Slice(keyed, func(i, j int) bool {
		a, b := keyed[i], keyed[j]
		if a.finding.Path != b.finding.Path {
			return a.finding.Path < b.finding.Path
		}
		if a.finding.StartLine != b.finding.StartLine {
			return a.finding.StartLine < b.finding.StartLine
		}
		if a.finding.RuleID != b.finding.RuleID {
			return a.finding.RuleID < b.finding.RuleID
		}
		if a.finding.Analyzer != b.finding.Analyzer {
			return a.finding.Analyzer < b.finding.Analyzer
		}
		if a.finding.Fingerprint != b.finding.Fingerprint {
			return a.finding.Fingerprint < b.finding.Fingerprint
		}
		return a.identity < b.identity
	})

	unique := make([]Finding, 0, len(keyed))
	lastIdentity := ""
	hasLast := false
	for _, item := range keyed {
		if hasLast && item.identity == lastIdentity {
			continue
		}
		unique = append(unique, item.finding)
		lastIdentity = item.identity
		hasLast = true
	}
	return unique, nil
}

func normalizeArtifactPath(uri, rootAbs string) (string, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" { return "", fmt.Errorf("empty artifact URI") }
	u, err := url.Parse(strings.ReplaceAll(uri, "\\", "/"))
	if err != nil { return "", fmt.Errorf("invalid artifact URI %q: %w", uri, err) }
	if u.Scheme != "" && u.Scheme != "file" {
		return "", fmt.Errorf("non-file artifact URI is not repository evidence: %q", uri)
	}
	if u.Scheme == "file" && u.Host != "" {
		return "", fmt.Errorf("file artifact URI must not specify a remote host: %q", uri)
	}
	// url.Parse already decodes u.Path. Do not unescape it again: a literal
	// filename containing the characters "%2e" must not be reinterpreted as "..".
	raw := u.Path
	raw = filepath.FromSlash(raw)
	if filepath.IsAbs(raw) {
		abs, err := filepath.Abs(raw)
		if err != nil { return "", fmt.Errorf("resolve artifact path: %w", err) }
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil { return "", fmt.Errorf("make artifact path repository-relative: %w", err) }
		raw = rel
	}
	clean := path.Clean(filepath.ToSlash(raw))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) ||
		(len(clean) >= 2 && clean[1] == ':') {
		return "", fmt.Errorf("artifact path escapes repository root: %q", uri)
	}
	return clean, nil
}

func findingFingerprint(raw sarifResult, f Finding) string {
	for _, key := range []string{"primaryLocationLineHash", "primaryLocationStartColumnFingerprint"} {
		if v := strings.TrimSpace(raw.PartialFingerprints[key]); v != "" { return v }
		if v := strings.TrimSpace(raw.Fingerprints[key]); v != "" { return v }
	}
	// Avoid including SARIF object formatting so the fallback remains stable
	// across serializer versions.
	return sha256Hex([]byte(strings.Join([]string{f.Analyzer, f.AnalyzerVersion, f.RuleID, f.Path,
		fmt.Sprint(f.StartLine), fmt.Sprint(f.StartColumn), f.Message}, "\x00")))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// WriteJSON emits deterministic JSON (finding and analyzer order is normalized
// by ParseSARIF). Output encoding errors are returned to the caller.
func WriteJSON(result ScanResult) ([]byte, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil { return nil, fmt.Errorf("encode normalized analyzer evidence: %w", err) }
	return append(data, '\n'), nil
}

// HashFile returns the SHA-256 digest of a configuration or input file.
func HashFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil { return "", fmt.Errorf("read %s for hashing: %w", file, err) }
	return sha256Hex(data), nil
}
