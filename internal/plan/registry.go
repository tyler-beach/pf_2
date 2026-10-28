package plan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/evidence"
)

// RunnerRegistry dispatches plan tasks to registered analyzers. Unknown names
// fail closed as unsupported.
type RunnerRegistry struct {
	Runners map[string]TaskRunner
}

func (r RunnerRegistry) Run(ctx context.Context, root string, task Task) (analyzer.ScanResult, error) {
	name := strings.ToLower(strings.TrimSpace(task.Analyzer))
	runner, ok := r.Runners[name]
	if !ok || runner == nil {
		return analyzer.ScanResult{}, &RunFailure{Kind: FailureUnsupported, Err: fmt.Errorf("no executor registered for analyzer %q", task.Analyzer)}
	}
	return runner.Run(ctx, root, task)
}

// DefaultRunnerRegistry registers OpenGrep, Checkov, and the native JSON-fact producer.
func DefaultRunnerRegistry(opengrepBinary string) RunnerRegistry {
	return DefaultRunnerRegistryWithBinaries(opengrepBinary, "checkov")
}

// DefaultRunnerRegistryWithBinaries registers analyzers with explicit CLI paths.
func DefaultRunnerRegistryWithBinaries(opengrepBinary, checkovBinary string) RunnerRegistry {
	if strings.TrimSpace(opengrepBinary) == "" {
		opengrepBinary = "opengrep"
	}
	if strings.TrimSpace(checkovBinary) == "" {
		checkovBinary = "checkov"
	}
	return RunnerRegistry{Runners: map[string]TaskRunner{
		"opengrep":  OpenGrepTaskRunner{Binary: opengrepBinary},
		"checkov":   CheckovTaskRunner{Binary: checkovBinary},
		"json-fact": JSONFactTaskRunner{},
	}}
}

// JSONFactTaskRunner collects configuration facts into a ScanResult envelope so
// execute-plan can register a second producer beside OpenGrep.
type JSONFactTaskRunner struct{}

func (JSONFactTaskRunner) Run(ctx context.Context, root string, task Task) (analyzer.ScanResult, error) {
	if ctx != nil && ctx.Err() != nil {
		return analyzer.ScanResult{}, &RunFailure{Kind: FailureTimeout, Err: ctx.Err()}
	}
	if !strings.EqualFold(strings.TrimSpace(task.Analyzer), "json-fact") {
		return analyzer.ScanResult{}, &RunFailure{Kind: FailureUnsupported, Err: fmt.Errorf("json-fact runner invoked for %q", task.Analyzer)}
	}
	configPath := task.ConfigurationPath
	if configPath == "" {
		return analyzer.ScanResult{}, &RunFailure{Kind: FailureAnalyzer, Err: fmt.Errorf("json-fact requires configuration_path pointing at a JSON file")}
	}
	producer := evidence.JSONFactProducer{}
	bundle, err := producer.Collect(root, configPath, task.ID)
	if err != nil {
		return analyzer.ScanResult{}, &RunFailure{Kind: FailureAnalyzer, Err: err}
	}
	version := task.Version
	if version == "" {
		version = evidence.JSONFactAdapterVersion
	}
	raw, err := minimalJSONFactSARIF(version, bundle)
	if err != nil {
		return analyzer.ScanResult{}, &RunFailure{Kind: FailureAnalyzer, Err: err}
	}
	sum := sha256.Sum256(raw)
	out := analyzer.ScanResult{
		SchemaVersion: "1", SARIFVersion: analyzer.SARIFVersion,
		Status:            analyzer.ScanStatusComplete,
		Analyzers:         []analyzer.AnalyzerInfo{{Name: "json-fact", Version: version}},
		ConfigurationPath: task.ConfigurationPath, ConfigurationSHA256: task.ConfigurationSHA256,
		BinarySHA256: task.BinarySHA256, RuleSetSHA256: task.RuleSetSHA256,
		InputSHA256: task.InputSHA256, RawOutputSHA256: hex.EncodeToString(sum[:]),
		ScannedPaths: append([]string(nil), bundle.ScannedPaths...),
		Findings:     []analyzer.Finding{}, RawSARIF: raw,
	}
	if task.BinarySHA256 == "" {
		out.BinarySHA256 = strings.Repeat("0", 64)
	}
	if bundle.ScanCompleteness == "ERROR" {
		out.Status = analyzer.ScanStatusError
		out.Errors = append([]string(nil), bundle.Errors...)
	}
	for _, rec := range bundle.Records {
		if rec.Kind != evidence.KindConfigValue {
			continue
		}
		key, _ := rec.Metadata["key"].(string)
		out.Findings = append(out.Findings, analyzer.Finding{
			Analyzer: "json-fact", AnalyzerVersion: version,
			RuleID: "json-fact.config-value", Message: fmt.Sprintf("observed configuration key %q", key),
			Path: rec.Artifact.Path, Level: "note", Fingerprint: rec.EvidenceID,
		})
	}
	return out, nil
}

func minimalJSONFactSARIF(version string, bundle evidence.Bundle) ([]byte, error) {
	type message struct {
		Text string `json:"text"`
	}
	type artifactLocation struct {
		URI string `json:"uri"`
	}
	type physicalLocation struct {
		ArtifactLocation artifactLocation `json:"artifactLocation"`
	}
	type location struct {
		PhysicalLocation physicalLocation `json:"physicalLocation"`
	}
	type result struct {
		RuleID       string            `json:"ruleId"`
		Level        string            `json:"level"`
		Message      message           `json:"message"`
		Locations    []location        `json:"locations"`
		Fingerprints map[string]string `json:"fingerprints"`
	}
	type rule struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type driver struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Rules   []rule `json:"rules"`
	}
	type tool struct {
		Driver driver `json:"driver"`
	}
	type run struct {
		Tool    tool     `json:"tool"`
		Results []result `json:"results"`
	}
	type document struct {
		Version string `json:"version"`
		Runs    []run  `json:"runs"`
	}
	results := make([]result, 0, len(bundle.Records))
	for _, rec := range bundle.Records {
		if rec.Kind != evidence.KindConfigValue {
			continue
		}
		key, _ := rec.Metadata["key"].(string)
		results = append(results, result{
			RuleID: "json-fact.config-value", Level: "note",
			Message:      message{Text: fmt.Sprintf("observed configuration key %q", key)},
			Locations:    []location{{PhysicalLocation: physicalLocation{ArtifactLocation: artifactLocation{URI: rec.Artifact.Path}}}},
			Fingerprints: map[string]string{"primary": rec.EvidenceID},
		})
	}
	doc := document{
		Version: analyzer.SARIFVersion,
		Runs: []run{{
			Tool:    tool{Driver: driver{Name: "json-fact", Version: version, Rules: []rule{{ID: "json-fact.config-value", Name: "config-value"}}}},
			Results: results,
		}},
	}
	return json.Marshal(doc)
}
