package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

const JSONFactAdapterVersion = "json-fact-1"

// JSONFactProducer emits configuration_value evidence records from JSON files.
// It proves a second producer can feed the same canonical envelope without
// changing the evaluator. Built-in JSON verifiers remain authoritative for PASS/FAIL.
type JSONFactProducer struct {
	RequiredKeys []string
	NumericKeys  []string
}

// Collect reads a repository-relative JSON file and emits observed configuration facts.
func (p JSONFactProducer) Collect(root, relativePath, runID string) (Bundle, error) {
	relativePath = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(relativePath))))
	if relativePath == "" || relativePath == "." || strings.HasPrefix(relativePath, "../") || filepath.IsAbs(filepath.FromSlash(relativePath)) {
		return Bundle{}, fmt.Errorf("json fact path must be repository-relative")
	}
	absolute := filepath.Join(root, filepath.FromSlash(relativePath))
	data, err := os.ReadFile(absolute)
	if err != nil {
		return Bundle{}, fmt.Errorf("read json fact source: %w", err)
	}
	fileHash := sha256.Sum256(data)
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		bundle := Bundle{
			SchemaVersion: SchemaVersion, ScanCompleteness: "ERROR",
			Producers: []analyzer.AnalyzerInfo{{Name: "json-fact", Version: JSONFactAdapterVersion}},
			Errors: []string{err.Error()},
			Records: []Record{{
				SchemaVersion: SchemaVersion, EvidenceID: stableID(runID, "parse-error", relativePath),
				AnalysisRunID: runID, Kind: KindLimitation, Status: StatusParseError,
				Producer: Producer{Analyzer: "json-fact", Version: JSONFactAdapterVersion, AdapterVersion: JSONFactAdapterVersion},
				Artifact: Artifact{Path: relativePath, SHA256: hex.EncodeToString(fileHash[:])},
				Diagnostics: []string{err.Error()},
			}},
		}
		return bundle, nil
	}
	bundle := Bundle{
		SchemaVersion: SchemaVersion, ScanCompleteness: "COMPLETE",
		Producers: []analyzer.AnalyzerInfo{{Name: "json-fact", Version: JSONFactAdapterVersion}},
		ScannedPaths: []string{relativePath},
		Records: make([]Record, 0, len(payload)),
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	wantKeys := map[string]bool{}
	for _, key := range p.RequiredKeys {
		wantKeys[key] = true
	}
	for _, key := range p.NumericKeys {
		wantKeys[key] = true
	}
	for _, key := range keys {
		if len(wantKeys) > 0 && !wantKeys[key] {
			continue
		}
		value := payload[key]
		id := stableID(runID, relativePath, key, fmt.Sprintf("%v", value))
		bundle.Records = append(bundle.Records, Record{
			SchemaVersion: SchemaVersion, EvidenceID: id, AnalysisRunID: runID,
			Producer: Producer{Analyzer: "json-fact", Version: JSONFactAdapterVersion, AdapterVersion: JSONFactAdapterVersion},
			Kind: KindConfigValue, Status: StatusObserved,
			Artifact: Artifact{Path: relativePath, SHA256: hex.EncodeToString(fileHash[:])},
			Metadata: map[string]any{"key": key, "value": value},
			Scope: Scope{Declared: []string{relativePath}, AnalyzedFiles: 1, ScannedPaths: []string{relativePath}},
		})
	}
	sort.SliceStable(bundle.Records, func(i, j int) bool {
		return bundle.Records[i].EvidenceID < bundle.Records[j].EvidenceID
	})
	return bundle, nil
}
