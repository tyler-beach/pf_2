package assessment

import (
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

func fixtureCatalog() Catalog {
	return Catalog{SchemaVersion:"1",ADRs:[]ADR{{
		ID:"ADR-001",Title:"Boundaries",Version:1,Status:"accepted",
		Requirements:[]Requirement{
			{ID:"REQ-001-01",Title:"No infrastructure imports",Severity:"error",Verifier:Verifier{Type:"go-import-boundary",SourcePathPrefixes:[]string{"internal/domain/"},ForbiddenImportPrefixes:[]string{"internal/infrastructure/"}}},
			{ID:"REQ-001-02",Title:"No HTTP imports",Severity:"error",Verifier:Verifier{Type:"go-import-boundary",SourcePathPrefixes:[]string{"internal/domain/"},ForbiddenImportPrefixes:[]string{"net/http"}}},
		},
	}}}
}
func TestAssessmentPassesCompliantSnippet(t *testing.T) {
	md:="### File: internal/domain/customer.go\n```go\npackage domain\nimport \"strings\"\nfunc Normalize(s string) string { return strings.TrimSpace(s) }\n```\n"
	result:=Assess(fixtureCatalog(),ParseIssueMarkdown(md))
	if result.Verdict!="PASS" { t.Fatalf("expected PASS, got %s: %+v",result.Verdict,result.Requirements) }
	if result.LLMCalls!=0 { t.Fatalf("expected zero LLM calls, got %d",result.LLMCalls) }
	for _,r:=range result.Requirements { if r.Status!="PASS" { t.Errorf("%s expected PASS, got %s",r.RequirementID,r.Status) } }
}
func TestAssessmentFindsMultipleDeviations(t *testing.T) {
	md:="### File: internal/domain/customer.go\n```go\npackage domain\nimport (\n \"internal/infrastructure/db\"\n \"net/http\"\n)\nfunc Load() {}\n```\n"
	result:=Assess(fixtureCatalog(),ParseIssueMarkdown(md))
	if result.Verdict!="FAIL" { t.Fatalf("expected FAIL, got %s",result.Verdict) }
	if result.Counts["FAIL"]!=2 { t.Fatalf("expected two failed requirements, counts=%v",result.Counts) }
	for _,r:=range result.Requirements { if len(r.Findings)!=1 { t.Errorf("%s expected one finding, got %+v",r.RequirementID,r.Findings) } }
}
func TestNoEvidenceIsMissing(t *testing.T) {
	result:=Assess(fixtureCatalog(),nil)
	if result.Verdict!="INDETERMINATE" { t.Fatalf("expected INDETERMINATE, got %s",result.Verdict) }
	for _,r:=range result.Requirements { if r.Status!="EVIDENCE_MISSING" { t.Errorf("%s expected EVIDENCE_MISSING, got %s",r.RequirementID,r.Status) } }
}
func TestCatalogRejectsDuplicateRequirementIDs(t *testing.T) {
	c:=fixtureCatalog(); c.ADRs[0].Requirements[1].ID=c.ADRs[0].Requirements[0].ID
	if err:=ValidateCatalog(c); err==nil || !strings.Contains(err.Error(),"duplicate requirement ID") { t.Fatalf("expected duplicate ID error, got %v",err) }
}
func TestImportPrefixBoundary(t *testing.T) {
	if !importMatches("internal/infrastructure","internal/infrastructure/") { t.Fatal("expected package root match") }
	if !importMatches("internal/infrastructure/db","internal/infrastructure/") { t.Fatal("expected child package match") }
	if importMatches("internal/infrastructurex/db","internal/infrastructure/") { t.Fatal("must not match sibling prefix") }
}

func jsonRequirementCatalog() Catalog {
	return Catalog{SchemaVersion:"1", ADRs:[]ADR{{
		ID:"ADR-002", Title:"Explicit runtime configuration", Version:1, Status:"accepted",
		Requirements:[]Requirement{{
			ID:"REQ-002-01", Title:"Require timeout and retry keys", Severity:"error",
			Verifier:Verifier{Type:"json-required-keys", FilePath:"config/service.json", RequiredJSONKeys:[]string{"timeout_seconds","retry_limit"}},
		}},
	}}}
}

func TestJSONRequiredKeysPassAndUsesZeroTokens(t *testing.T) {
	md := "### File: config/service.json\n```json\n{\n  \"timeout_seconds\": 30,\n  \"retry_limit\": 3\n}\n```\n"
	files := ParseIssueMarkdown(md)
	if len(files)!=1 || files[0].Language!="json" { t.Fatalf("expected one parsed JSON file, got %+v", files) }
	result := Assess(jsonRequirementCatalog(), files)
	if result.Verdict!="PASS" { t.Fatalf("expected PASS, got %s: %+v", result.Verdict, result.Requirements) }
	if result.LLMCalls!=0 { t.Fatalf("expected zero LLM calls, got %d", result.LLMCalls) }
}

func TestJSONRequiredKeysFindMissingKey(t *testing.T) {
	md := "### File: config/service.json\n```json\n{\n  \"timeout_seconds\": 30\n}\n```\n"
	result := Assess(jsonRequirementCatalog(), ParseIssueMarkdown(md))
	if result.Verdict!="FAIL" { t.Fatalf("expected FAIL, got %s", result.Verdict) }
	if len(result.Requirements)!=1 || result.Requirements[0].Status!="FAIL" { t.Fatalf("expected requirement failure, got %+v", result.Requirements) }
	if len(result.Requirements[0].Findings)!=1 || !strings.Contains(result.Requirements[0].Findings[0].Message,"retry_limit") { t.Fatalf("expected missing retry_limit finding, got %+v", result.Requirements[0].Findings) }
}

func TestJSONRequiredKeysMalformedInputIsError(t *testing.T) {
	files := []SourceFile{{Path:"config/service.json", Language:"json", Content:"{ invalid json"}}
	result := Assess(jsonRequirementCatalog(), files)
	if result.Requirements[0].Status!="ERROR" { t.Fatalf("expected ERROR, got %+v", result.Requirements[0]) }
}

func TestJSONRequiredKeysMissingFileIsEvidenceMissing(t *testing.T) {
	files := []SourceFile{{Path:"config/other.json", Language:"json", Content:"{}"}}
	result := Assess(jsonRequirementCatalog(), files)
	if result.Requirements[0].Status!="EVIDENCE_MISSING" { t.Fatalf("expected EVIDENCE_MISSING, got %+v", result.Requirements[0]) }
	if result.Verdict != "INDETERMINATE" { t.Fatalf("missing required file must not pass, got %s", result.Verdict) }
}

func TestCatalogRejectsUnsafeJSONVerifierPath(t *testing.T) {
	c := jsonRequirementCatalog()
	c.ADRs[0].Requirements[0].Verifier.FilePath = "../outside.json"
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "repository-relative file_path") {
		t.Fatalf("expected unsafe path validation error, got %v", err)
	}
}

func TestCatalogRejectsJSONVerifierWithoutRequiredKeys(t *testing.T) {
	c := jsonRequirementCatalog()
	c.ADRs[0].Requirements[0].Verifier.RequiredJSONKeys = nil
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "required_json_keys") {
		t.Fatalf("expected missing key list validation error, got %v", err)
	}
}

func numericRangeCatalog() Catalog {
	min, max := 1.0, 120.0
	return Catalog{SchemaVersion:"1", ADRs:[]ADR{{
		ID:"ADR-003", Title:"Bound configuration values", Version:1, Status:"accepted",
		Requirements:[]Requirement{{
			ID:"REQ-003-01", Title:"Timeout must be bounded", Severity:"error",
			Verifier:Verifier{Type:"json-number-range", FilePath:"config/service.json", NumericJSONBounds:[]JSONNumberBound{{Key:"timeout_seconds", Min:&min, Max:&max}}},
		}},
	}}}
}
func TestJSONNumberRangePassesInclusiveBounds(t *testing.T) {
	md := "### File: config/service.json\n```json\n{ \"timeout_seconds\": 120 }\n```\n"
	result := Assess(numericRangeCatalog(), ParseIssueMarkdown(md))
	if result.Verdict != "PASS" || result.LLMCalls != 0 { t.Fatalf("expected zero-token PASS, got %+v", result) }
}
func TestJSONNumberRangeRejectsOutOfRangeValues(t *testing.T) {
	for _, input := range []string{`{"timeout_seconds":0}`, `{"timeout_seconds":121}`} {
		files := []SourceFile{{Path:"config/service.json", Language:"json", Content:input}}
		result := Assess(numericRangeCatalog(), files)
		if result.Verdict != "FAIL" || len(result.Requirements[0].Findings) != 1 { t.Errorf("expected one numeric range finding for %s, got %+v", input, result) }
	}
}
func TestJSONNumberRangeRejectsMissingAndNonNumericValues(t *testing.T) {
	for _, input := range []string{`{}`, `{"timeout_seconds":"30"}`} {
		result := Assess(numericRangeCatalog(), []SourceFile{{Path:"config/service.json", Language:"json", Content:input}})
		if result.Verdict != "FAIL" || len(result.Requirements[0].Findings) != 1 { t.Errorf("expected one type/presence finding for %s, got %+v", input, result) }
	}
}
func TestJSONNumberRangeRejectsInvalidBounds(t *testing.T) {
	c := numericRangeCatalog()
	min, max := 121.0, 1.0
	c.ADRs[0].Requirements[0].Verifier.NumericJSONBounds[0] = JSONNumberBound{Key:"timeout_seconds", Min:&min, Max:&max}
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "min greater than max") { t.Fatalf("expected invalid bound rejection, got %v", err) }
}

func TestCoverageIsPartialWhenAcceptedRequirementsHaveNoMatchingEvidence(t *testing.T) {
	c := jsonRequirementCatalog()
	c.ADRs = append(c.ADRs, fixtureCatalog().ADRs...)
	files := []SourceFile{{Path:"config/service.json", Language:"json", Content:`{"timeout_seconds":30,"retry_limit":3}`}}
	result := Assess(c, files)
	if result.Verdict != "PASS" { t.Fatalf("expected scoped PASS among applicable requirements, got %s", result.Verdict) }
	if result.CoverageStatus != "PARTIAL" { t.Fatalf("expected PARTIAL coverage, got %s", result.CoverageStatus) }
	if result.ApplicableRequirementCount != 1 || result.RequirementCount != 3 { t.Fatalf("unexpected coverage counts: applicable=%d total=%d", result.ApplicableRequirementCount, result.RequirementCount) }
}
func TestRepositoryAssessmentDistinguishesAbsentScopeFromMissingRepositoryEvidence(t *testing.T) {
	c := fixtureCatalog()
	nonGo := []SourceFile{{Path:"README.md", Language:"markdown", Content:"# Docs"}}
	result := AssessWithOptions(c, nonGo, AssessmentOptions{EvidenceMode: EvidenceModeRepository})
	for _, requirement := range result.Requirements {
		if requirement.Status != "NOT_APPLICABLE" {
			t.Errorf("%s status = %s, want NOT_APPLICABLE for confirmed empty Go scope", requirement.RequirementID, requirement.Status)
		}
	}
	empty := AssessWithOptions(c, nil, AssessmentOptions{EvidenceMode: EvidenceModeRepository})
	for _, requirement := range empty.Requirements {
		if requirement.Status != "EVIDENCE_MISSING" {
			t.Errorf("%s status = %s, want EVIDENCE_MISSING when no evidence was discovered", requirement.RequirementID, requirement.Status)
		}
	}
	if empty.Verdict != "INDETERMINATE" {
		t.Fatalf("empty repository evidence must be INDETERMINATE, got %s", empty.Verdict)
	}
}

func TestCoverageIsCompleteWhenAllAcceptedRequirementsAreEvaluated(t *testing.T) {
	c := fixtureCatalog()
	files := []SourceFile{{Path:"internal/domain/customer.go", Language:"go", Content:"package domain\nimport \"strings\"\n"}}
	result := Assess(c, files)
	if result.CoverageStatus != "COMPLETE" || result.ApplicableRequirementCount != result.RequirementCount { t.Fatalf("expected complete coverage, got %+v", result) }
}


func analyzerRuleCatalog() Catalog {
	return Catalog{SchemaVersion: "1", ADRs: []ADR{{
		ID: "ADR-003", Title: "Avoid dynamic shell execution", Version: 1, Status: "accepted",
		Requirements: []Requirement{{
			ID: "REQ-003-01", Title: "Do not invoke shell with dynamic command strings", Severity: "error",
			Verifier: Verifier{Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"pathfinder.go-shell-command"}, SourcePathPrefixes: []string{"cmd/", "internal/"}},
		}},
	}}}
}

func completeAnalyzerEvidence(findings []analyzer.Finding) analyzer.ScanResult {
	return analyzer.ScanResult{
		SchemaVersion: "1", SARIFVersion: "2.1.0", Status: analyzer.ScanStatusComplete,
		Analyzers: []analyzer.AnalyzerInfo{{Name: "Opengrep OSS Version", Version: "1.30.0"}},
		ConfigurationSHA256: strings.Repeat("c", 64), InputSHA256: strings.Repeat("i", 64),
		RawOutputSHA256: strings.Repeat("r", 64),
		ScannedPaths: []string{"cmd/adrctl/main.go", "internal/analyzer/opengrep.go", "testdata/analyzer-fixtures/vulnerable/shell_command.go"},
		Findings: findings,
	}
}

func TestAnalyzerRulePassesOnlyWithCompleteScopedEvidence(t *testing.T) {
	result := AssessWithOptions(analyzerRuleCatalog(), []SourceFile{{Path: "cmd/adrctl/main.go", Language: "go"}}, AssessmentOptions{
		EvidenceMode: EvidenceModeRepository, AnalyzerResults: []analyzer.ScanResult{completeAnalyzerEvidence(nil)},
	})
	if result.Verdict != "PASS" { t.Fatalf("expected PASS, got %+v", result) }
	requirement := result.Requirements[0]
	if requirement.Status != "PASS" || len(requirement.Evidence) == 0 { t.Fatalf("missing analyzer evidence: %+v", requirement) }
	if !strings.Contains(requirement.Evidence[0], strings.Repeat("i", 64)) || !strings.Contains(requirement.Evidence[0], strings.Repeat("c", 64)) {
		t.Fatalf("input/config provenance not surfaced: %+v", requirement.Evidence)
	}
	if result.AnalyzerResults[0].Status != analyzer.ScanStatusComplete { t.Fatal("normalized analyzer run not retained in result") }
}

func TestAnalyzerRuleFailsAndPreservesFindingProvenance(t *testing.T) {
	finding := analyzer.Finding{Analyzer: "Opengrep OSS Version", AnalyzerVersion: "1.30.0", RuleID: "pathfinder.go-shell-command",
		Message: "Avoid dynamically composed shell command", Path: "internal/app/runner.go",
		StartLine: 17, StartColumn: 3, Level: "error", Fingerprint: "fingerprint-123",
		Properties: map[string]any{"cwe": "CWE-78"}}
	result := AssessWithOptions(analyzerRuleCatalog(), nil, AssessmentOptions{
		EvidenceMode: EvidenceModeRepository, AnalyzerResults: []analyzer.ScanResult{completeAnalyzerEvidence([]analyzer.Finding{finding})},
	})
	if result.Verdict != "FAIL" || result.Requirements[0].Status != "FAIL" { t.Fatalf("expected FAIL, got %+v", result) }
	got := result.Requirements[0].Findings[0]
	if got.Path != finding.Path || got.Line != 17 || got.Column != 3 || got.Fingerprint != "fingerprint-123" || got.AnalyzerVersion != "1.30.0" {
		t.Fatalf("finding provenance was lost: %+v", got)
	}
}

func TestAnalyzerRuleFiltersFindingsOutsideDeclaredScope(t *testing.T) {
	finding := analyzer.Finding{Analyzer: "opengrep", RuleID: "pathfinder.go-shell-command", Message: "fixture violation", Path: "testdata/analyzer-fixtures/vulnerable/shell_command.go", StartLine: 4}
	result := AssessWithOptions(analyzerRuleCatalog(), nil, AssessmentOptions{
		EvidenceMode: EvidenceModeRepository, AnalyzerResults: []analyzer.ScanResult{completeAnalyzerEvidence([]analyzer.Finding{finding})},
	})
	if result.Verdict != "PASS" || result.Requirements[0].Status != "PASS" { t.Fatalf("out-of-scope fixture should not fail the policy: %+v", result) }
}

func TestAnalyzerRuleMissingOrPartialEvidenceIsIndeterminate(t *testing.T) {
	partial := completeAnalyzerEvidence(nil); partial.Status = analyzer.ScanStatusPartial; partial.Errors = []string{"timeout"}
	missingHash := completeAnalyzerEvidence(nil); missingHash.RawOutputSHA256 = ""
	emptyInventory := completeAnalyzerEvidence(nil); emptyInventory.ScannedPaths = nil
	cases := map[string][]analyzer.ScanResult{"missing": nil, "partial": {partial}, "missing hashes": {missingHash}, "empty inventory": {emptyInventory}}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			result := AssessWithOptions(analyzerRuleCatalog(), nil, AssessmentOptions{EvidenceMode: EvidenceModeRepository, AnalyzerResults: evidence})
			if result.Verdict != "INDETERMINATE" || result.Requirements[0].Status != "EVIDENCE_MISSING" {
				t.Fatalf("incomplete evidence must not pass: %+v", result)
			}
		})
	}
}

func TestAnalyzerRuleRequiresPathForMatchingFinding(t *testing.T) {
	finding := analyzer.Finding{Analyzer: "opengrep", RuleID: "pathfinder.go-shell-command", Message: "unlocated finding"}
	result := AssessWithOptions(analyzerRuleCatalog(), nil, AssessmentOptions{
		EvidenceMode: EvidenceModeRepository, AnalyzerResults: []analyzer.ScanResult{completeAnalyzerEvidence([]analyzer.Finding{finding})},
	})
	if result.Verdict != "INDETERMINATE" || result.Requirements[0].Status != "EVIDENCE_MISSING" {
		t.Fatalf("matching finding without path must fail closed: %+v", result)
	}
}

func TestAnalyzerRuleCatalogValidation(t *testing.T) {
	c := analyzerRuleCatalog()
	if err := ValidateCatalog(c); err != nil { t.Fatalf("valid analyzer rule rejected: %v", err) }
	c.ADRs[0].Requirements[0].Verifier.RuleIDs = nil
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "rule_ids") { t.Fatalf("expected rule id validation error, got %v", err) }
	c = analyzerRuleCatalog()
	c.ADRs[0].Requirements[0].Verifier.SourcePathPrefixes = []string{"../outside"}
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "repository-relative") { t.Fatalf("expected scope path validation error, got %v", err) }
}

func TestSubmissionModeDoesNotClaimRepositoryAnalyzerWasEvaluated(t *testing.T) {
	result := Assess(analyzerRuleCatalog(), []SourceFile{{Path: "cmd/adrctl/main.go", Language: "go"}})
	if result.Requirements[0].Status != "NOT_APPLICABLE" { t.Fatalf("submission mode should not pretend to run scanner: %+v", result.Requirements[0]) }
	if result.Verdict != "INDETERMINATE" { t.Fatalf("without any passing/failed submitted-scope check, verdict should remain INDETERMINATE, got %s", result.Verdict) }
}
