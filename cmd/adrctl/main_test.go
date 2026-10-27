package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

func TestEndToEndReviewedADRToRepositoryAssessment(t *testing.T) {
	root := t.TempDir()
	adrRoot := filepath.Join(root, "adrs")
	if err := os.MkdirAll(adrRoot, 0o700); err != nil { t.Fatal(err) }
	adrSource := "# ADR-001: Preserve domain boundaries\n\n## Status\n\nAccepted\n\n## Decision\n\nDomain code must not import internal infrastructure packages.\n"
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), []byte(adrSource), 0o600); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(root, "internal", "domain"), 0o700); err != nil { t.Fatal(err) }
	sourcePath := filepath.Join(root, "internal", "domain", "customer.go")
	if err := os.WriteFile(sourcePath, []byte("package domain\nimport \"strings\"\nfunc Normalize(s string) string { return strings.TrimSpace(s) }\n"), 0o600); err != nil { t.Fatal(err) }

	catalog := assessment.Catalog{SchemaVersion: "1", ADRs: []assessment.ADR{{
		ID: "ADR-001", Title: "Preserve domain boundaries", Version: 1, Status: "accepted",
		Requirements: []assessment.Requirement{{
			ID: "REQ-001-01", Title: "Domain code must not depend on infrastructure adapters",
			Description: "Domain source files must not import internal infrastructure packages.",
			Severity: "error", Verifier: assessment.Verifier{
				Type: "go-import-boundary", SourcePathPrefixes: []string{"internal/domain/"},
				ForbiddenImportPrefixes: []string{"internal/infrastructure/"},
			},
		}},
	}}}
	catalogPath := filepath.Join(root, "adr", "requirements.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o700); err != nil { t.Fatal(err) }
	catalogBytes, err := json.Marshal(catalog)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(catalogPath, catalogBytes, 0o600); err != nil { t.Fatal(err) }

	ingestionPath := filepath.Join(root, "adr", "ingested-adrs.json")
	if err := runIngestADRs([]string{"--root", adrRoot, "--json-out", ingestionPath}); err != nil { t.Fatal(err) }
	candidatePath := filepath.Join(root, "adr", "obligation-candidates.json")
	if err := runExtractObligations([]string{"--index", ingestionPath, "--root", adrRoot, "--json-out", candidatePath}); err != nil { t.Fatal(err) }
	index, err := readObligationIndex(candidatePath)
	if err != nil { t.Fatal(err) }
	if len(index.Candidates) != 1 { t.Fatalf("candidate count=%d, want 1: %+v", len(index.Candidates), index) }
	candidate := index.Candidates[0]

	registryPath := filepath.Join(root, "adr", "obligation-reviews.json")
	if err := runInitReviewRegistry([]string{"--output", registryPath}); err != nil { t.Fatal(err) }
	proposalPath := filepath.Join(t.TempDir(), "review-proposal.json")
	proposal := obligations.ReviewProposal{
		CandidateID: candidate.ID, Decision: obligations.DecisionApproved,
		RequirementID: "REQ-001-01", PatternID: candidate.PatternHint,
		EnforcementMode: obligations.EnforcementBlocking,
		Reviewer: "security-architect@example.com", ReviewedAt: "2026-10-27T12:00:00Z",
		ReviewReference: "https://github.com/tyler-beach/pf_2/issues/900",
		Rationale: "The reviewed boundary obligation matches the existing deterministic Go import verifier.",
	}
	proposalBytes, err := json.Marshal(proposal)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(proposalPath, proposalBytes, 0o600); err != nil { t.Fatal(err) }
	if err := runRecordObligationReview([]string{
		"--index", candidatePath, "--requirements", catalogPath, "--registry", registryPath, "--proposal", proposalPath,
	}); err != nil { t.Fatal(err) }

	assessmentPath := filepath.Join(root, "assessment.json")
	repoAssessmentArgs := []string{
		"--root", root, "--requirements", catalogPath,
		"--report", filepath.Join(root, "report.md"), "--json-out", assessmentPath,
		"--sarif-out", filepath.Join(root, "results.sarif"),
	}
	if err := runAssess(repoAssessmentArgs, true); err != nil { t.Fatal(err) }
	jsonReport := filepath.Join(root, "obligation-assessment.json")
	markdownReport := filepath.Join(root, "obligation-assessment.md")
	args := []string{
		"--root", root, "--index", candidatePath, "--ingestion-index", ingestionPath, "--requirements", catalogPath,
		"--registry", registryPath, "--assessment", assessmentPath,
		"--json-out", jsonReport, "--report", markdownReport,
	}
	if err := runAssessObligations(args); err != nil { t.Fatal(err) }
	readReport := func() obligations.AssessmentReport {
		data, err := os.ReadFile(jsonReport)
		if err != nil { t.Fatal(err) }
		var report obligations.AssessmentReport
		if err := json.Unmarshal(data, &report); err != nil { t.Fatal(err) }
		return report
	}
	passed := readReport()
	if !passed.EvidenceFresh || passed.Candidates[0].Status != "PASS" ||
		passed.Candidates[0].Decision != obligations.DecisionApproved ||
		passed.Candidates[0].RequirementID != "REQ-001-01" {
		t.Fatalf("reviewed obligation did not complete the end-to-end pass path: %+v", passed)
	}

	// A fresh repository scan with a real boundary violation should fail.
	if err := os.WriteFile(sourcePath, []byte("package domain\nimport \"internal/infrastructure/storage\"\n"), 0o600); err != nil { t.Fatal(err) }
	if err := runAssess(repoAssessmentArgs, true); err != nil { t.Fatal(err) }
	if err := runAssessObligations(args); err != nil { t.Fatal(err) }
	failed := readReport()
	if !failed.EvidenceFresh || failed.Candidates[0].Status != "FAIL" || failed.Candidates[0].EvidenceStatus != "FAIL" {
		t.Fatalf("fresh failing repository evidence was not reported as FAIL: %+v", failed)
	}

	// Mutating source again without rerunning assessment must not preserve FAIL.
	if err := os.WriteFile(sourcePath, []byte("package domain\nfunc Normalize(s string) string { return s }\n"), 0o600); err != nil { t.Fatal(err) }
	if err := runAssessObligations(args); err != nil { t.Fatal(err) }
	stale := readReport()
	if stale.EvidenceFresh || stale.Candidates[0].Status != "INDETERMINATE" || stale.Candidates[0].EvidenceStatus != "STALE" {
		t.Fatalf("stale assessment remained compliant after source changed: %+v", stale)
	}

	// An ADR edit must invalidate the extracted candidate index itself.
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), append([]byte(adrSource), '\n'), 0o600); err != nil { t.Fatal(err) }
	if err := runAssessObligations(args); err == nil || !strings.Contains(err.Error(), "rebuild candidate index") {
		t.Fatalf("stale candidate index should be rejected after an ADR edit, got %v", err)
	}
}

func TestRunAssessObligationsWritesIndeterminateForUnreviewedCandidate(t *testing.T) {
	root := t.TempDir()
	adrRoot := filepath.Join(root, "adrs")
	if err := os.MkdirAll(adrRoot, 0o700); err != nil { t.Fatal(err) }
	source := []byte("# ADR-001: Explicit timeout policy\n\n## Status\n\nAccepted\n\n## Decision\n\nThe service must set a finite timeout and must keep it below 30 seconds.\n")
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), source, 0o600); err != nil { t.Fatal(err) }
	doc, err := adringest.Parse("ADR-001.md", source)
	if err != nil { t.Fatal(err) }
	ingestionPath := filepath.Join(root, "adr", "ingested-adrs.json")
	if err := runIngestADRs([]string{"--root", adrRoot, "--json-out", ingestionPath}); err != nil { t.Fatal(err) }
	indexPath := filepath.Join(root, "adr", "obligation-candidates.json")
	if err := runExtractObligations([]string{"--index", ingestionPath, "--root", adrRoot, "--json-out", indexPath}); err != nil { t.Fatal(err) }
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil { t.Fatal(err) }
	var index obligations.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil { t.Fatal(err) }
	candidates := index.Candidates

	catalog := assessment.Catalog{SchemaVersion: "1", ADRs: []assessment.ADR{{
		ID: doc.ID, Title: doc.Title, Version: 1, Status: "accepted", Requirements: []assessment.Requirement{},
	}}}
	catalogPath := filepath.Join(root, "requirements.json")
	catalogBytes, err := json.Marshal(catalog)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(catalogPath, catalogBytes, 0o600); err != nil { t.Fatal(err) }

	registryPath := filepath.Join(root, "reviews.json")
	if err := obligations.SaveRegistry(registryPath, obligations.NewRegistry()); err != nil { t.Fatal(err) }
	assessmentPath := filepath.Join(root, "assessment.json")
	sourceFiles, err := assessment.LoadSourceFiles(root)
	if err != nil { t.Fatal(err) }
	evidenceBytes, err := json.Marshal(assessment.Result{
		SchemaVersion: "1", EvidenceMode: string(assessment.EvidenceModeRepository),
		Verdict: "PASS", Requirements: []assessment.RequirementResult{},
		SourceSnapshotSHA256: assessment.HashSourceFiles(sourceFiles),
	})
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(assessmentPath, evidenceBytes, 0o600); err != nil { t.Fatal(err) }

	jsonPath := filepath.Join(root, "obligation-assessment.json")
	markdownPath := filepath.Join(root, "obligation-assessment.md")
	if err := runAssessObligations([]string{
		"--index", indexPath, "--ingestion-index", ingestionPath, "--requirements", catalogPath, "--registry", registryPath,
		"--assessment", assessmentPath, "--root", root, "--json-out", jsonPath, "--report", markdownPath,
	}); err != nil { t.Fatal(err) }
	data, err := os.ReadFile(jsonPath)
	if err != nil { t.Fatal(err) }
	var report obligations.AssessmentReport
	if err := json.Unmarshal(data, &report); err != nil { t.Fatal(err) }
	if report.CandidateCount != 1 || report.Counts["INDETERMINATE"] != 1 || report.Candidates[0].Status != "INDETERMINATE" {
		t.Fatalf("unreviewed obligation was not reported as indeterminate: %+v", report)
	}
	if !report.EvidenceFresh { t.Fatalf("matching source snapshot was incorrectly marked stale: %+v", report) }
	if len(report.Candidates[0].ReviewHints) != 1 || report.Candidates[0].ReviewHints[0] != "compound_or_conjoined" {
		t.Fatalf("candidate interpretation hints were lost: %+v", report.Candidates[0])
	}
	registryBytes, err := os.ReadFile(registryPath)
	if err != nil { t.Fatal(err) }
	indexDigest, registryDigest, assessmentDigest := sha256.Sum256(indexBytes), sha256.Sum256(registryBytes), sha256.Sum256(evidenceBytes)
	if report.CandidateIndexArtifactSHA256 != hex.EncodeToString(indexDigest[:]) ||
		report.ReviewRegistryArtifactSHA256 != hex.EncodeToString(registryDigest[:]) ||
		report.AssessmentArtifactSHA256 != hex.EncodeToString(assessmentDigest[:]) {
		t.Fatalf("report did not preserve exact input artifact digests: %+v", report)
	}
	markdown, err := os.ReadFile(markdownPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(markdown), "INDETERMINATE") || !strings.Contains(string(markdown), candidates[0].ID) || !strings.Contains(string(markdown), "compound_or_conjoined") {
		t.Fatalf("Markdown report is missing obligation status or identity: %s", markdown)
	}

	// Change an input without changing its decoded meaning. The original
	// assessment artifact must be treated as stale against the current tree.
	if err := os.WriteFile(catalogPath, append(catalogBytes, '\n'), 0o600); err != nil { t.Fatal(err) }
	if err := runAssessObligations([]string{
		"--index", indexPath, "--ingestion-index", ingestionPath, "--requirements", catalogPath, "--registry", registryPath,
		"--assessment", assessmentPath, "--root", root, "--json-out", jsonPath, "--report", markdownPath,
	}); err != nil { t.Fatal(err) }
	staleBytes, err := os.ReadFile(jsonPath)
	if err != nil { t.Fatal(err) }
	var stale obligations.AssessmentReport
	if err := json.Unmarshal(staleBytes, &stale); err != nil { t.Fatal(err) }
	if stale.EvidenceFresh || stale.RepositoryVerdict != "INDETERMINATE" || stale.Candidates[0].Status != "INDETERMINATE" {
		t.Fatalf("assessment was not downgraded after its source snapshot changed: %+v", stale)
	}
}

func TestRunIngestADRsWritesDeterministicIndex(t *testing.T) {
	root := t.TempDir()
	adrRoot := filepath.Join(root, "adrs")
	if err := os.MkdirAll(adrRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	source := "# ADR-001: Use explicit timeouts\n\n## Status\n\nAccepted\n\n## Decision\n\nSet a timeout on every outbound request.\n"
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "generated", "index.json")
	if err := runIngestADRs([]string{"--root", adrRoot, "--json-out", output}); err != nil {
		t.Fatalf("runIngestADRs() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var index ingestionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("invalid output JSON: %v", err)
	}
	if index.SchemaVersion != "1" || len(index.Documents) != 1 {
		t.Fatalf("unexpected index: %+v", index)
	}
	if index.Documents[0].ID != "ADR-001" || index.Documents[0].DecisionText == "" {
		t.Fatalf("document was not ingested: %+v", index.Documents[0])
	}
	if !strings.Contains(string(data), "source_sha256") {
		t.Fatal("index should preserve source hash provenance")
	}
}

func TestRunIngestADRsRejectsDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"ADR-001.md", "another-copy.md"} {
		source := "# ADR-001: Duplicate ID\n\n## Decision\n\nUse explicit timeouts.\n"
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := runIngestADRs([]string{"--root", root, "--json-out", filepath.Join(t.TempDir(), "index.json")})
	if err == nil || !strings.Contains(err.Error(), "duplicate ADR ID") {
		t.Fatalf("expected duplicate ADR ID error, got %v", err)
	}
}

func TestRunIngestADRsRejectsMissingRoot(t *testing.T) {
	err := runIngestADRs([]string{"--root", filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected missing root error")
	}
}

func TestIngestionIndexDocumentsHaveProvenance(t *testing.T) {
	doc, err := adringest.Parse("ADR-9.md", []byte("# ADR-9: Example\n\n## Decision\n\nDo the thing.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.SourceSHA256) != 64 || doc.Sections[0].StartLine != 3 {
		t.Fatalf("unexpected provenance: %+v", doc)
	}
}


func TestRunAssessRepoConsumesAnalyzerEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0700); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "cmd", "main.go"), []byte("package main\n"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, "rules", "security.yml"), []byte("rules: []\n"), 0600); err != nil { t.Fatal(err) }
	catalog := assessment.Catalog{SchemaVersion: "1", ADRs: []assessment.ADR{{
		ID: "ADR-003", Title: "Analyzer policy", Version: 1, Status: "accepted",
		Requirements: []assessment.Requirement{{
			ID: "REQ-003-01", Title: "No shell execution", Severity: "error",
			Verifier: assessment.Verifier{Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"rule.no-shell"}, SourcePathPrefixes: []string{"cmd/"}},
		}},
	}}}
	catalogPath := filepath.Join(root, "requirements.json")
	catalogBytes, err := json.Marshal(catalog)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(catalogPath, catalogBytes, 0600); err != nil { t.Fatal(err) }

	raw := []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"opengrep","version":"1.30.0"}},"results":[]}]}`)
	evidence, err := analyzer.ParseSARIF(raw, root)
	if err != nil { t.Fatal(err) }
	configHash, err := analyzer.HashFile(filepath.Join(root, "rules", "security.yml"))
	if err != nil { t.Fatal(err) }
	inputHash, err := analyzer.HashInputTree(root)
	if err != nil { t.Fatal(err) }
	evidence.ConfigurationPath = "rules/security.yml"
	evidence.ConfigurationSHA256 = configHash
	evidence.BinarySHA256 = strings.Repeat("a", 64)
	evidence.RuleSetSHA256 = configHash
	evidence.InputSHA256 = inputHash
	evidence.ScannedPaths = []string{"cmd/main.go"}
	evidenceRoot := t.TempDir()
	evidencePath := filepath.Join(evidenceRoot, "opengrep.json")
	evidenceBytes, err := json.Marshal(evidence)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(evidencePath, evidenceBytes, 0600); err != nil { t.Fatal(err) }
	sarifPath := filepath.Join(evidenceRoot, "opengrep.sarif")
	if err := os.WriteFile(sarifPath, raw, 0600); err != nil { t.Fatal(err) }
	resultPath := filepath.Join(root, "assessment.json")
	if err := runAssess([]string{"--root", root, "--requirements", catalogPath,
		"--report", filepath.Join(root, "report.md"), "--json-out", resultPath,
		"--sarif-out", filepath.Join(root, "results.sarif"), "--analyzer-evidence", evidencePath,
		"--analyzer-sarif", sarifPath}, true); err != nil {
		t.Fatalf("runAssess() error = %v", err)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil { t.Fatal(err) }
	var got assessment.Result
	if err := json.Unmarshal(data, &got); err != nil { t.Fatal(err) }
	if got.Verdict != "PASS" || len(got.AnalyzerResults) != 1 { t.Fatalf("analyzer evidence was not included: %+v", got) }
	if len(got.AnalyzerResults[0].ScannedPaths) == 0 { t.Fatalf("verified scan inventory missing: %+v", got.AnalyzerResults[0]) }
}

func TestRunAssessRejectsNormalizedEvidenceWithoutRawSARIF(t *testing.T) {
	root := t.TempDir()
	catalog, err := assessment.LoadCatalog(filepath.Join("..", "..", "adr", "requirements.json"))
	if err != nil { t.Fatal(err) }
	catalogBytes, err := json.Marshal(catalog)
	if err != nil { t.Fatal(err) }
	catalogPath := filepath.Join(root, "requirements.json")
	if err := os.WriteFile(catalogPath, catalogBytes, 0600); err != nil { t.Fatal(err) }
	evidenceRoot := t.TempDir()
	evidencePath := filepath.Join(evidenceRoot, "evidence.json")
	if err := os.WriteFile(evidencePath, []byte(`{"schema_version":"1","sarif_version":"2.1.0","status":"COMPLETE"}`), 0600); err != nil { t.Fatal(err) }
	err = runAssess([]string{"--root", root, "--requirements", catalogPath,
		"--report", filepath.Join(root, "report.md"), "--json-out", filepath.Join(root, "assessment.json"),
		"--analyzer-evidence", evidencePath}, true)
	if err == nil || !strings.Contains(err.Error(), "--analyzer-sarif is required") {
		t.Fatalf("expected missing raw SARIF rejection, got %v", err)
	}
}

func TestRunExtractObligationsWritesReviewOnlyCandidatesDeterministically(t *testing.T) {
	root := t.TempDir()
	adrRoot := filepath.Join(root, "adrs")
	if err := os.MkdirAll(adrRoot, 0700); err != nil { t.Fatal(err) }
	source := "# ADR-001: Explicit timeouts\n\n## Status\n\nAccepted\n\n## Decision\n\nEvery network client must set a finite timeout.\n"
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), []byte(source), 0600); err != nil { t.Fatal(err) }
	docs, err := adringest.ParseDir(adrRoot)
	if err != nil { t.Fatal(err) }
	indexBytes, err := json.MarshalIndent(ingestionIndex{SchemaVersion:"1", SourceRoot:adrRoot, Documents:docs}, "", "  ")
	if err != nil { t.Fatal(err) }
	indexPath := filepath.Join(root, "ingested.json")
	if err := os.WriteFile(indexPath, indexBytes, 0600); err != nil { t.Fatal(err) }
	outPath := filepath.Join(root, "candidates.json")
	reportPath := filepath.Join(root, "candidates.md")
	args := []string{"--index", indexPath, "--root", adrRoot, "--json-out", outPath, "--report", reportPath}
	if err := runExtractObligations(args); err != nil { t.Fatalf("runExtractObligations() error: %v", err) }
	first, err := os.ReadFile(outPath)
	if err != nil { t.Fatal(err) }
	var got obligations.Index
	if err := json.Unmarshal(first, &got); err != nil { t.Fatal(err) }
	if got.DocumentCount != 1 || got.CandidateCount != 1 || got.Candidates[0].ApprovalStatus != "pending_review" || got.Candidates[0].Executable {
		t.Fatalf("candidate output was not review-only: %+v", got)
	}
	reportBytes, err := os.ReadFile(reportPath)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(reportBytes), got.Candidates[0].ID) || !strings.Contains(string(reportBytes), "pending_review") || !strings.Contains(string(reportBytes), "Every network client must set a finite timeout") {
		t.Fatalf("candidate Markdown report is missing review details: %s", reportBytes)
	}
	secondPath := filepath.Join(root, "candidates-again.json")
	secondReportPath := filepath.Join(root, "candidates-again.md")
	args[5] = secondPath
	args[7] = secondReportPath
	if err := runExtractObligations(args); err != nil { t.Fatalf("second run: %v", err) }
	second, err := os.ReadFile(secondPath)
	if err != nil { t.Fatal(err) }
	if string(first) != string(second) { t.Fatalf("candidate generation changed across identical inputs:\n%s\n%s", first, second) }
	secondReport, err := os.ReadFile(secondReportPath)
	if err != nil { t.Fatal(err) }
	if string(reportBytes) != string(secondReport) { t.Fatalf("candidate review report changed across identical inputs:\n%s\n%s", reportBytes, secondReport) }
}

func TestRunExtractObligationsRefusesToOverwriteIndex(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, "index.json")
	if err := os.WriteFile(indexPath, []byte("{"), 0600); err != nil { t.Fatal(err) }
	err := runExtractObligations([]string{"--index", indexPath, "--root", root, "--json-out", indexPath})
	if err == nil || !strings.Contains(err.Error(), "must not overwrite") { t.Fatalf("expected index overwrite rejection, got %v", err) }
}


func TestRunAssertIssueVerdict(t *testing.T) {
	root := t.TempDir()
	reportPath := filepath.Join(root, "assessment.json")
	writeResult := func(verdict, coverage string) {
		t.Helper()
		data, err := json.Marshal(assessment.Result{SchemaVersion:"1", Verdict:verdict, CoverageStatus:coverage})
		if err != nil { t.Fatal(err) }
		if err := os.WriteFile(reportPath, data, 0600); err != nil { t.Fatal(err) }
	}
	writeResult("INDETERMINATE", "PARTIAL")
	if err := runAssertIssueVerdict([]string{"--title", "[ADR Assessment] [expected:INDETERMINATE] missing source scope", "--assessment", reportPath}); err != nil {
		t.Fatalf("matching expected verdict failed: %v", err)
	}
	if err := runAssertIssueVerdict([]string{"--title", "[ADR Assessment] no assertion", "--assessment", reportPath}); err != nil {
		t.Fatalf("unmarked issue title should not assert a verdict: %v", err)
	}
	if err := runAssertIssueVerdict([]string{"--title", "[ADR Assessment] [expected:PASS] missing source scope", "--assessment", reportPath}); err == nil || !strings.Contains(err.Error(), "expected PASS") {
		t.Fatalf("mismatched verdict should fail, got %v", err)
	}
	if err := runAssertIssueVerdict([]string{"--assessment", reportPath}); err == nil || !strings.Contains(err.Error(), "--title is required") {
		t.Fatalf("missing title should fail, got %v", err)
	}
}


func TestResolveOpenGrepBinaryDigest(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "opengrep-test")
	if err := os.WriteFile(binary, []byte("deterministic test executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := analyzer.ResolveExecutable(binary)
	if err != nil {
		t.Fatal(err)
	}
	want, err := analyzer.HashFile(resolved)
	if err != nil {
		t.Fatal(err)
	}

	gotPath, gotDigest, err := resolveOpenGrepBinary(binary, "")
	if err != nil {
		t.Fatalf("omitted expected digest should calculate actual digest: %v", err)
	}
	if gotPath != resolved || gotDigest != want {
		t.Fatalf("resolved identity mismatch: path=%q digest=%q; want path=%q digest=%q", gotPath, gotDigest, resolved, want)
	}

	gotPath, gotDigest, err = resolveOpenGrepBinary(binary, want)
	if err != nil || gotPath != resolved || gotDigest != want {
		t.Fatalf("matching expected digest was rejected: path=%q digest=%q err=%v", gotPath, gotDigest, err)
	}
	if _, _, err := resolveOpenGrepBinary(binary, strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected digest mismatch to fail closed, got %v", err)
	}
	if _, _, err := resolveOpenGrepBinary(binary, "not-a-digest"); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("invalid expected digest should be rejected, got %v", err)
	}
	if _, _, err := resolveOpenGrepBinary(filepath.Join(t.TempDir(), "missing-opengrep"), ""); err == nil {
		t.Fatal("missing executable unexpectedly resolved")
	}
}
