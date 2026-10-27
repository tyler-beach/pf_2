// Package evidence defines the vendor-neutral, versioned evidence envelope.
// Analyzers normalize into this contract; the evaluator consumes it without
// depending on SARIF or a specific tool's output shape.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

const SchemaVersion = "1.0"

// Record statuses describe observation quality, not ADR verdicts.
const (
	StatusObserved    = "observed"
	StatusAbsent      = "absent-within-declared-scope"
	StatusUnsupported = "unsupported"
	StatusParseError  = "parse-error"
	StatusToolError   = "tool-error"
	StatusIncomplete  = "incomplete"
)

// Kinds distinguish scanner alerts from structural facts and limitations.
const (
	KindScannerFinding = "scanner_finding"
	KindLimitation     = "analysis_limitation"
	KindConfigValue    = "configuration_value"
	KindSyntaxFact     = "syntax_fact"
)

// Producer identifies the analyzer/adapter that emitted a record.
type Producer struct {
	Analyzer       string `json:"analyzer"`
	Version        string `json:"version,omitempty"`
	AdapterVersion string `json:"adapter_version,omitempty"`
	CompositeID    string `json:"composite_id,omitempty"`
	RuleID         string `json:"rule_id,omitempty"`
	RulesetDigest  string `json:"ruleset_digest,omitempty"`
	ConfigDigest   string `json:"config_digest,omitempty"`
	BinaryDigest   string `json:"binary_digest,omitempty"`
}

// Artifact locates the subject of an observation in the repository snapshot.
type Artifact struct {
	Path        string `json:"path,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Language    string `json:"language,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	StartColumn int    `json:"start_column,omitempty"`
}

// Finding carries scanner-alert metadata when Kind is scanner_finding.
type Finding struct {
	ID       string `json:"id"`
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Provenance ties a record to raw tool output for audit/reprocessing.
type Provenance struct {
	RawArtifactURI    string `json:"raw_artifact_uri,omitempty"`
	RawArtifactSHA256 string `json:"raw_artifact_sha256,omitempty"`
	ExtractionMethod  string `json:"extraction_method,omitempty"`
}

// Scope describes what the producer claimed to analyze.
type Scope struct {
	Declared         []string `json:"declared,omitempty"`
	AnalyzedFiles    int      `json:"analyzed_files,omitempty"`
	UnsupportedFiles int      `json:"unsupported_files,omitempty"`
	ScannedPaths     []string `json:"scanned_paths,omitempty"`
}

// Record is one normalized observation or limitation.
type Record struct {
	SchemaVersion string         `json:"schema_version"`
	EvidenceID    string         `json:"evidence_id"`
	Repository    string         `json:"repository,omitempty"`
	Revision      string         `json:"revision,omitempty"`
	AnalysisRunID string         `json:"analysis_run_id,omitempty"`
	Producer      Producer       `json:"producer"`
	Kind          string         `json:"kind"`
	Artifact      Artifact       `json:"artifact,omitempty"`
	Finding       *Finding       `json:"finding,omitempty"`
	Status        string         `json:"status"`
	Provenance    Provenance     `json:"provenance,omitempty"`
	Scope         Scope          `json:"scope,omitempty"`
	Limitations   []string       `json:"limitations,omitempty"`
	Diagnostics   []string       `json:"diagnostics,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// Bundle groups records from one or more producers for a single assessment.
type Bundle struct {
	SchemaVersion string   `json:"schema_version"`
	Revision      string   `json:"revision,omitempty"`
	Records       []Record `json:"records"`
	// ScanCompleteness mirrors analyzer.ScanResult status for fail-closed gates.
	ScanCompleteness string                  `json:"scan_completeness,omitempty"`
	Producers        []analyzer.AnalyzerInfo `json:"producers,omitempty"`
	InputSHA256      string                  `json:"input_sha256,omitempty"`
	RawOutputSHA256  string                  `json:"raw_output_sha256,omitempty"`
	ConfigPath       string                  `json:"configuration_path,omitempty"`
	ConfigSHA256     string                  `json:"configuration_sha256,omitempty"`
	BinarySHA256     string                  `json:"binary_sha256,omitempty"`
	RuleSetSHA256    string                  `json:"rule_set_sha256,omitempty"`
	ScannedPaths     []string                `json:"scanned_paths,omitempty"`
	Errors           []string                `json:"errors,omitempty"`
}

// FromScanResult adapts OpenGrep/SARIF-normalized ScanResult into the canonical envelope.
// Raw SARIF remains the audit artifact; this function never invents PASS/FAIL verdicts.
func FromScanResult(scan analyzer.ScanResult, runID string) Bundle {
	bundle := Bundle{
		SchemaVersion:    SchemaVersion,
		Revision:         scan.InputSHA256,
		ScanCompleteness: scan.Status,
		Producers:        append([]analyzer.AnalyzerInfo(nil), scan.Analyzers...),
		InputSHA256:      scan.InputSHA256,
		RawOutputSHA256:  scan.RawOutputSHA256,
		ConfigPath:       scan.ConfigurationPath,
		ConfigSHA256:     scan.ConfigurationSHA256,
		BinarySHA256:     scan.BinarySHA256,
		RuleSetSHA256:    scan.RuleSetSHA256,
		ScannedPaths:     append([]string(nil), scan.ScannedPaths...),
		Errors:           append([]string(nil), scan.Errors...),
		Records:          make([]Record, 0, len(scan.Findings)+len(scan.Errors)),
	}
	status := StatusObserved
	switch scan.Status {
	case analyzer.ScanStatusPartial:
		status = StatusIncomplete
	case analyzer.ScanStatusError:
		status = StatusToolError
	}
	scope := Scope{
		Declared:      []string{"."},
		AnalyzedFiles: len(scan.ScannedPaths),
		ScannedPaths:  append([]string(nil), scan.ScannedPaths...),
	}
	for _, errMsg := range scan.Errors {
		id := stableID(runID, "limitation", errMsg, "", 0)
		bundle.Records = append(bundle.Records, Record{
			SchemaVersion: SchemaVersion,
			EvidenceID:    id,
			Revision:      scan.InputSHA256,
			AnalysisRunID: runID,
			Producer: Producer{
				Analyzer: firstAnalyzerName(scan), Version: firstAnalyzerVersion(scan),
				AdapterVersion: "opengrep-sarif", RulesetDigest: digestURI(scan.RuleSetSHA256),
				ConfigDigest: digestURI(scan.ConfigurationSHA256), BinaryDigest: digestURI(scan.BinarySHA256),
			},
			Kind:   KindLimitation,
			Status: status,
			Provenance: Provenance{
				RawArtifactSHA256: digestURI(scan.RawOutputSHA256),
				ExtractionMethod:  "sarif-normalize",
			},
			Scope:       scope,
			Limitations: []string{errMsg},
			Diagnostics: []string{errMsg},
		})
	}
	for _, finding := range scan.Findings {
		id := finding.Fingerprint
		if id == "" {
			id = stableID(runID, finding.RuleID, finding.Message, finding.Path, finding.StartLine)
		}
		producerName := finding.Analyzer
		if producerName == "" {
			producerName = firstAnalyzerName(scan)
		}
		producerVersion := finding.AnalyzerVersion
		if producerVersion == "" {
			producerVersion = firstAnalyzerVersion(scan)
		}
		rec := Record{
			SchemaVersion: SchemaVersion,
			EvidenceID:    id,
			Revision:      scan.InputSHA256,
			AnalysisRunID: runID,
			Producer: Producer{
				Analyzer: producerName, Version: producerVersion, AdapterVersion: "opengrep-sarif",
				RuleID: finding.RuleID, RulesetDigest: digestURI(scan.RuleSetSHA256),
				ConfigDigest: digestURI(scan.ConfigurationSHA256), BinaryDigest: digestURI(scan.BinarySHA256),
			},
			Kind: KindScannerFinding,
			Artifact: Artifact{
				Path:      filepathToSlash(finding.Path),
				StartLine: finding.StartLine, StartColumn: finding.StartColumn,
			},
			Finding: &Finding{
				ID: id, RuleID: finding.RuleID, Severity: finding.Level, Message: finding.Message,
			},
			Status: StatusObserved,
			Provenance: Provenance{
				RawArtifactSHA256: digestURI(scan.RawOutputSHA256),
				ExtractionMethod:  "sarif-normalize",
			},
			Scope: scope,
		}
		if scan.Status != analyzer.ScanStatusComplete {
			rec.Status = status
		}
		bundle.Records = append(bundle.Records, rec)
	}
	sort.SliceStable(bundle.Records, func(i, j int) bool {
		return bundle.Records[i].EvidenceID < bundle.Records[j].EvidenceID
	})
	return bundle
}

// BundlesFromScans converts a slice of ScanResults into envelopes.
func BundlesFromScans(scans []analyzer.ScanResult) []Bundle {
	out := make([]Bundle, 0, len(scans))
	for i, scan := range scans {
		out = append(out, FromScanResult(scan, fmt.Sprintf("scan-%d", i+1)))
	}
	return out
}

// StableEvidenceIDs returns sorted evidence IDs for reproducibility tests.
func StableEvidenceIDs(bundle Bundle) []string {
	ids := make([]string, 0, len(bundle.Records))
	for _, rec := range bundle.Records {
		ids = append(ids, rec.EvidenceID)
	}
	sort.Strings(ids)
	return ids
}

func firstAnalyzerName(scan analyzer.ScanResult) string {
	if len(scan.Analyzers) == 0 {
		return "unknown"
	}
	return scan.Analyzers[0].Name
}

func firstAnalyzerVersion(scan analyzer.ScanResult) string {
	if len(scan.Analyzers) == 0 {
		return ""
	}
	return scan.Analyzers[0].Version
}

func digestURI(hexDigest string) string {
	hexDigest = strings.ToLower(strings.TrimSpace(hexDigest))
	if hexDigest == "" {
		return ""
	}
	return "sha256:" + hexDigest
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func stableID(parts ...any) string {
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteByte(0)
		}
		fmt.Fprintf(&b, "%v", part)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// MarshalCanonical encodes a bundle with stable key ordering for digests.
func MarshalCanonical(bundle Bundle) ([]byte, error) {
	return json.Marshal(bundle)
}
