package orchestrate

import (
	"path/filepath"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
)

func TestDefaultOrchestratorAssessesFixture(t *testing.T) {
	root := filepath.Join("..", "..")
	orch := NewDefaultOrchestrator()
	result, err := orch.Assess(filepath.Join(root, "adr", "requirements.json"), []assessment.SourceFile{{
		Path: "internal/domain/customer.go", Language: "go",
		Content: "package domain\n",
	}, {
		Path: "config/service.json", Language: "json",
		Content: `{"timeout_seconds":30,"retry_limit":3}`,
	}}, assessment.AssessmentOptions{EvidenceMode: assessment.EvidenceModeSubmission})
	if err != nil {
		t.Fatal(err)
	}
	if result.LLMCalls != 0 {
		t.Fatalf("runtime LLM calls = %d", result.LLMCalls)
	}
	if result.CorpusCoverage.AcceptedADRs == 0 {
		t.Fatal("expected accepted ADR inventory in corpus coverage")
	}
}

func TestScanNormalizerUsesEvidenceEnvelope(t *testing.T) {
	var n Normalizer = ScanNormalizer{}
	bundle := n.Normalize(analyzer.ScanResult{
		SchemaVersion: "1", Status: analyzer.ScanStatusComplete,
		Analyzers:       []analyzer.AnalyzerInfo{{Name: "opengrep", Version: "1.30.0"}},
		InputSHA256:     "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RawOutputSHA256: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		ScannedPaths:    []string{"a.go"},
		Findings: []analyzer.Finding{{
			RuleID: "rules.pathfinder.go-shell-command", Path: "a.go", StartLine: 1,
			Fingerprint: "fp", Analyzer: "opengrep",
		}},
	}, "run-1")
	if len(bundle.Records) != 1 {
		t.Fatalf("records = %d", len(bundle.Records))
	}
}
