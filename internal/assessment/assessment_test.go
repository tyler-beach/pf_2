package assessment

import (
	"os"
	"path/filepath"
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
func TestHashSourceFilesIsOrderIndependentAndContentSensitive(t *testing.T) {
	files := []SourceFile{
		{Path: "internal/a.go", Language: "go", Content: "package a"},
		{Path: "config/service.json", Language: "json", Content: `{"timeout_seconds":30}`},
	}
	first := HashSourceFiles(files)
	reversed := HashSourceFiles([]SourceFile{files[1], files[0]})
	if first != reversed || len(first) != 64 {
		t.Fatalf("source snapshot digest is not deterministic: %s vs %s", first, reversed)
	}
	changed := []SourceFile{files[0], {Path: files[1].Path, Language: files[1].Language, Content: `{"timeout_seconds":31}`}}
	if HashSourceFiles(changed) == first {
		t.Fatal("source content change did not change the snapshot digest")
	}
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
	if result.Verdict != "INDETERMINATE" { t.Fatalf("missing issue source scope must not be treated as PASS, got %s", result.Verdict) }
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
			Verifier: Verifier{Type: "analyzer-rule", Analyzer: "opengrep", RuleIDs: []string{"pathfinder.go-shell-command"}, SourcePathPrefixes: []string{"cmd/", "internal/"}, SourceExtensions: []string{".go"}},
		}},
	}}}
}

func completeAnalyzerEvidence(findings []analyzer.Finding) analyzer.ScanResult {
	return analyzer.ScanResult{
		SchemaVersion: "1", SARIFVersion: "2.1.0", Status: analyzer.ScanStatusComplete,
		Analyzers: []analyzer.AnalyzerInfo{{Name: "Opengrep OSS Version", Version: "1.30.0"}},
		ConfigurationSHA256: strings.Repeat("c", 64), InputSHA256: strings.Repeat("i", 64),
		BinarySHA256: strings.Repeat("a", 64), RuleSetSHA256: strings.Repeat("b", 64),
		RawOutputSHA256: strings.Repeat("r", 64),
		ScannedPaths: []string{"cmd/adrctl/main.go", "internal/analyzer/opengrep.go", "internal/app/runner.go", "testdata/analyzer-fixtures/vulnerable/shell_command.go"},
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
	missingBinaryHash := completeAnalyzerEvidence(nil); missingBinaryHash.BinarySHA256 = ""
	missingRuleHash := completeAnalyzerEvidence(nil); missingRuleHash.RuleSetSHA256 = ""
	incompleteInventory := completeAnalyzerEvidence(nil); incompleteInventory.ScannedPaths = nil
	cases := map[string][]analyzer.ScanResult{
		"missing": nil, "partial": {partial}, "missing hashes": {missingHash},
		"missing binary digest": {missingBinaryHash}, "missing rule-set digest": {missingRuleHash},
		"empty inventory": {incompleteInventory},
	}
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


func TestIssueSnippetPathsAreSanitized(t *testing.T) {
	cases := []struct{ input, want string }{
		{"src/app.go", "src/app.go"},
		{"src/app|unsafe.go", "src/app_unsafe.go"},
		{"../../outside.go", "issue-snippet-1.go"},
		{"C:\\outside.go", "issue-snippet-1.go"},
		{"src/`bad`.go", "src/_bad_.go"},
	}
	for _, tc := range cases {
		md := "### File: " + tc.input + "\n```go\npackage fixture\n```\n"
		files := ParseIssueMarkdown(md)
		if len(files) != 1 { t.Fatalf("input %q produced %d files", tc.input, len(files)) }
		if files[0].Path != tc.want { t.Errorf("input %q path = %q, want %q", tc.input, files[0].Path, tc.want) }
		if filepath.IsAbs(files[0].Path) || strings.HasPrefix(files[0].Path, "../") || strings.ContainsAny(files[0].Path, "\r\n|`") {
			t.Errorf("unsafe path retained for %q: %q", tc.input, files[0].Path)
		}
	}
}

func FuzzParseIssueMarkdown(f *testing.F) {
	f.Add("### File: internal/domain/example.go\n```go\npackage domain\n```\n")
	f.Add("### File: ../../outside.json\n```json\n{}\n```\n")
	f.Add("### File: src/app|bad.go\n```go\npackage src\n```\n")
	f.Add("arbitrary ``` fenced text")
	f.Fuzz(func(t *testing.T, markdown string) {
		if len(markdown) > 1<<20 { t.Skip() }
		for _, file := range ParseIssueMarkdown(markdown) {
			if filepath.IsAbs(file.Path) || strings.HasPrefix(file.Path, "../") || strings.ContainsAny(file.Path, "\r\n|`") {
				t.Fatalf("unsafe issue source path: %q", file.Path)
			}
		}
	})
}

func TestLoadSourceFilesExcludesGeneratedEvidenceAndCaches(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"cmd/main.go": "package main\\n",
		"config/service.json": "{}\\n",
		"analyzer-evidence/opengrep.json": "{}\\n",
		"analyzer-evidence/untrusted.go": "package ignored\\n",
		".opengrep-cache/generated.go": "package ignored\\n",
		"assessment.json": "{}\\n",
		"adr/ingested-adrs.json": "{}\\n",
		"adr/obligation-candidates.json": "{}\\n",
		"testdata/fixture.go": "package ignored\\n",
	}
	for path, body := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil { t.Fatal(err) }
		if err := os.WriteFile(full, []byte(body), 0600); err != nil { t.Fatal(err) }
	}
	got, err := LoadSourceFiles(root)
	if err != nil { t.Fatal(err) }
	var paths []string
	for _, file := range got { paths = append(paths, file.Path) }
	if strings.Join(paths, ",") != "cmd/main.go,config/service.json" {
		t.Fatalf("source loader included generated artifacts or skipped source files: %v", paths)
	}
}


func TestPathMatchesRespectsPathSegmentBoundaries(t *testing.T) {
	cases := []struct{ path, prefix string; want bool }{
		{"cmd/runner.go", "cmd/", true},
		{"cmd", "cmd/", true},
		{"cmdx/runner.go", "cmd/", false},
		{"internal/domainx/file.go", "internal/domain/", false},
		{"internal/domain/file.go", "internal/domain/", true},
		{"internal/domain/file.go", "internal/domain", true},
		{"../outside.go", ".", false},
		{"internal/domain/file.go", ".", true},
	}
	for _, tc := range cases {
		if got := pathMatches(tc.path, []string{tc.prefix}); got != tc.want {
			t.Errorf("pathMatches(%q, %q) = %v, want %v", tc.path, tc.prefix, got, tc.want)
		}
	}
}

func TestLoadSourceFilesSkipsSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.go")
	if err := os.WriteFile(outside, []byte("package secret\n"), 0600); err != nil { t.Fatal(err) }
	inside := filepath.Join(root, "inside.go")
	if err := os.WriteFile(inside, []byte("package inside\n"), 0600); err != nil { t.Fatal(err) }
	linkPath := filepath.Join(root, "linked.go")
	if err := os.Symlink(outside, linkPath); err != nil { t.Skipf("symlink not available: %v", err) }
	files, err := LoadSourceFiles(root)
	if err != nil { t.Fatal(err) }
	if len(files) != 1 || files[0].Path != "inside.go" { t.Fatalf("symlink should not be followed: %+v", files) }
}

func TestLoadSourceFilesRejectsOversizedEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.go")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(maxSourceFileBytes+1))), 0600); err != nil { t.Fatal(err) }
	_, err := LoadSourceFiles(root)
	if err == nil || !strings.Contains(err.Error(), "exceeds the") { t.Fatalf("expected evidence size limit error, got %v", err) }
}

func TestLoadSourceFilesRequiresDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-dir")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil { t.Fatal(err) }
	_, err := LoadSourceFiles(file)
	if err == nil || !strings.Contains(err.Error(), "not a directory") { t.Fatalf("expected root directory validation error, got %v", err) }
}


func TestParseIssueMarkdownRecognizesMultipleLanguages(t *testing.T) {
	fence := strings.Repeat(string(rune(96)), 3)
	markdown := strings.Join([]string{
		"### File: src/service.py", fence + "python", "value = eval(input())", fence,
		"### File: src/client.ts", fence + "typescript", "const x: string = 'safe';", fence,
		"### File: src/Runner.java", fence + "java", "class Runner {}", fence,
		"### File: src/Program.cs", fence + "csharp", "class Program {}", fence,
		"### File: src/worker.rb", fence + "ruby", "puts 'safe'", fence,
		"### File: config/service.json", fence + "json", "{}", fence,
	}, "\n")
	files := ParseIssueMarkdown(markdown)
	if len(files) != 6 { t.Fatalf("parsed %d files, want 6: %+v", len(files), files) }
	want := map[string]string{
		"src/service.py": "python", "src/client.ts": "typescript", "src/Runner.java": "java",
		"src/Program.cs": "csharp", "src/worker.rb": "ruby", "config/service.json": "json",
	}
	for _, file := range files {
		if got := want[file.Path]; got == "" || got != file.Language {
			t.Errorf("unexpected parsed source %q language=%q want=%q", file.Path, file.Language, got)
		}
	}
}

func TestUnknownLanguageSubmissionIsExplicitlyIndeterminate(t *testing.T) {
	fence := strings.Repeat(string(rune(96)), 3)
	markdown := strings.Join([]string{
		"### File: src/record.cobol", fence + "cobol", "DISPLAY 'HELLO'.", fence,
		"### File: config/service.json", fence + "json", "{\"timeout_seconds\":30,\"retry_limit\":3}", fence,
	}, "\n")
	files := ParseIssueMarkdown(markdown)
	if len(files) != 2 { t.Fatalf("unknown-language file was silently dropped: %+v", files) }
	if files[0].Language != "unsupported:cobol" { t.Fatalf("unknown language not preserved: %+v", files) }
	result := Assess(jsonRequirementCatalog(), files)
	if result.Verdict != "INDETERMINATE" || result.CoverageStatus != "PARTIAL" {
		t.Fatalf("unassessed snippets must prevent PASS, got verdict=%s coverage=%s", result.Verdict, result.CoverageStatus)
	}
	if len(result.UnassessedSourceFiles) != 1 || !strings.Contains(result.UnassessedSourceFiles[0], "src/record.cobol") {
		t.Fatalf("unassessed evidence not identified: %+v", result.UnassessedSourceFiles)
	}
}

func TestNonGoSubmissionCannotPassOnlyBecauseNonGoCodeWasIgnored(t *testing.T) {
	fence := strings.Repeat(string(rune(96)), 3)
	markdown := strings.Join([]string{"### File: src/service.py", fence + "python", "value = eval(input())", fence}, "\n")
	result := Assess(fixtureCatalog(), ParseIssueMarkdown(markdown))
	if result.Verdict != "INDETERMINATE" || len(result.UnassessedSourceFiles) != 1 {
		t.Fatalf("non-Go issue evidence must not be ignored or pass, got %+v", result)
	}
}


func TestAnalyzerRuleRejectsFindingsAbsentFromScanInventory(t *testing.T) {
	finding := analyzer.Finding{Analyzer:"opengrep", RuleID:"pathfinder.go-shell-command", Message:"phantom finding", Path:"internal/phantom.go", StartLine:10}
	result := AssessWithOptions(analyzerRuleCatalog(), nil, AssessmentOptions{
		EvidenceMode: EvidenceModeRepository,
		AnalyzerResults: []analyzer.ScanResult{completeAnalyzerEvidence([]analyzer.Finding{finding})},
	})
	if result.Verdict != "INDETERMINATE" || result.Requirements[0].Status != "EVIDENCE_MISSING" {
		t.Fatalf("finding missing from scan inventory must not be trusted: %+v", result)
	}
}


func TestSafeSubmissionPathSanitizesFallbackExtension(t *testing.T) {
	unsafeExtension := filepath.Ext("../../outside." + string(rune(96)))
	got := safeSubmissionPath("../../outside."+string(rune(96)), 1, unsafeExtension)
	if got != "issue-snippet-1.txt" {
		t.Fatalf("unsafe extension escaped fallback path: %q", got)
	}
}


func TestAnalyzerRuleIsNotApplicableWhenCompletedInventoryHasNoMatchingExtension(t *testing.T) {
	c := analyzerRuleCatalog()
	c.ADRs[0].Requirements[0].Verifier.SourceExtensions = []string{".py"}
	result := AssessWithOptions(c, nil, AssessmentOptions{
		EvidenceMode: EvidenceModeRepository,
		AnalyzerResults: []analyzer.ScanResult{completeAnalyzerEvidence(nil)},
	})
	if result.Requirements[0].Status != "NOT_APPLICABLE" {
		t.Fatalf("no Python files in a complete inventory should be NOT_APPLICABLE, got %+v", result.Requirements[0])
	}
	if result.Verdict != "INDETERMINATE" || result.CoverageStatus != "NONE" {
		t.Fatalf("non-applicable-only assessment must not claim PASS, got verdict=%s coverage=%s", result.Verdict, result.CoverageStatus)
	}
	if len(result.Requirements[0].Evidence) == 0 { t.Fatal("non-applicability should cite verified inventory evidence") }
}

func TestAnalyzerRuleCatalogValidatesSourceExtensions(t *testing.T) {
	c := analyzerRuleCatalog()
	if err := ValidateCatalog(c); err != nil { t.Fatalf("valid analyzer rule rejected: %v", err) }
	c.ADRs[0].Requirements[0].Verifier.SourceExtensions = []string{"../outside"}
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "invalid source extension") {
		t.Fatalf("expected invalid source extension rejection, got %v", err)
	}
	c = analyzerRuleCatalog()
	c.ADRs[0].Requirements[0].Verifier.SourceExtensions = []string{".go", ".GO"}
	if err := ValidateCatalog(c); err == nil || !strings.Contains(err.Error(), "duplicate source extension") {
		t.Fatalf("expected duplicate source extension rejection, got %v", err)
	}
}


func TestSubmissionModeMissingGoScopeIsEvidenceMissing(t *testing.T) {
	files := []SourceFile{{
		Path: "config/service.json",
		Language: "json",
		Content: `{"timeout_seconds":30,"retry_limit":3}`,
	}}
	result := AssessWithOptions(fixtureCatalog(), files, AssessmentOptions{EvidenceMode: EvidenceModeSubmission})
	if result.Verdict != "INDETERMINATE" {
		t.Fatalf("incomplete issue snippets must not pass, got verdict=%s", result.Verdict)
	}
	for _, requirement := range result.Requirements {
		if requirement.Status != "EVIDENCE_MISSING" {
			t.Errorf("%s status = %s, want EVIDENCE_MISSING when submission contains no Go file in required scope", requirement.RequirementID, requirement.Status)
		}
	}
	if result.Counts["EVIDENCE_MISSING"] != len(result.Requirements) {
		t.Fatalf("every Go boundary check should report missing evidence, got counts=%v", result.Counts)
	}
}


func TestRollupFailureTakesPrecedenceOverMissingEvidence(t *testing.T) {
	result := rollup([]RequirementResult{
		{RequirementID: "REQ-MISSING", Status: "EVIDENCE_MISSING"},
		{RequirementID: "REQ-FAIL", Status: "FAIL"},
	})
	if result != "FAIL" { t.Fatalf("confirmed violation should take precedence over missing evidence, got %s", result) }
}

func TestRollupDoesNotPassWhenAnyApplicableEvidenceIsMissing(t *testing.T) {
	result := rollup([]RequirementResult{
		{RequirementID: "REQ-PASS", Status: "PASS"},
		{RequirementID: "REQ-MISSING", Status: "EVIDENCE_MISSING"},
		{RequirementID: "REQ-NA", Status: "NOT_APPLICABLE"},
	})
	if result != "INDETERMINATE" { t.Fatalf("incomplete evidence should prevent PASS, got %s", result) }
}


func TestUnsupportedVerifierIsExplicitAndCannotPass(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: "1",
		ADRs: []ADR{{
			ID: "ADR-UNSUPPORTED",
			Title: "Unsupported policy",
			Version: 1,
			Status: "accepted",
			Requirements: []Requirement{{
				ID: "REQ-UNSUPPORTED-01",
				Title: "Apply a policy engine that is not installed",
				Severity: "error",
				Verifier: Verifier{Type: "rego-bundle-v1"},
			}},
		}},
	}
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("unknown verifier type should remain assessable as unsupported: %v", err)
	}

	result := AssessWithOptions(catalog, nil, AssessmentOptions{EvidenceMode: EvidenceModeRepository})
	if len(result.Requirements) != 1 {
		t.Fatalf("got %d requirements, want 1", len(result.Requirements))
	}
	if got := result.Requirements[0].Status; got != "UNSUPPORTED" {
		t.Fatalf("unknown verifier status = %s, want UNSUPPORTED; result=%+v", got, result.Requirements[0])
	}
	if result.Counts["UNSUPPORTED"] != 1 {
		t.Fatalf("unsupported status not reflected in counts: %v", result.Counts)
	}
	if result.Verdict != "INDETERMINATE" {
		t.Fatalf("unsupported verifier must not establish compliance, got %s", result.Verdict)
	}
}

func TestEmptyCatalogCannotClaimCompliance(t *testing.T) {
	catalog := Catalog{SchemaVersion: "1", ADRs: []ADR{}}
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("empty schema-valid catalog should be explicitly assessed rather than misreported as compliance: %v", err)
	}
	result := AssessWithOptions(catalog, nil, AssessmentOptions{EvidenceMode: EvidenceModeRepository})
	if result.RequirementCount != 0 || len(result.Requirements) != 0 {
		t.Fatalf("empty catalog unexpectedly evaluated requirements: %+v", result.Requirements)
	}
	if result.CoverageStatus != "NONE" {
		t.Fatalf("empty catalog coverage = %s, want NONE", result.CoverageStatus)
	}
	if result.Verdict != "INDETERMINATE" {
		t.Fatalf("empty catalog must not claim PASS, got %s", result.Verdict)
	}
}


func TestManualReviewVerifierIsSupportedButCannotEstablishCompliance(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: "1",
		ADRs: []ADR{{
			ID: "ADR-REVIEW",
			Title: "Runtime suitability",
			Version: 1,
			Status: "accepted",
			Requirements: []Requirement{
				{
					ID: "REQ-REVIEW-01",
					Title: "The timeout must suit the service's business latency budget",
					Severity: "warning",
					Verifier: Verifier{Type: "manual-review"},
				},
				{
					ID: "REQ-REVIEW-02",
					Title: "Domain code must not depend on infrastructure adapters",
					Severity: "error",
					Verifier: Verifier{
						Type: "go-import-boundary",
						SourcePathPrefixes: []string{"internal/domain/"},
						ForbiddenImportPrefixes: []string{"internal/infrastructure/"},
					},
				},
			},
		}},
	}
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("manual-review verifier should be a supported catalog type: %v", err)
	}

	files := []SourceFile{{
		Path: "internal/domain/customer.go",
		Language: "go",
		Content: "package domain\nimport \"strings\"\nfunc Normalize(s string) string { return strings.TrimSpace(s) }\n",
	}}
	result := AssessWithOptions(catalog, files, AssessmentOptions{EvidenceMode: EvidenceModeRepository})
	if len(result.Requirements) != 2 {
		t.Fatalf("got %d requirements, want 2", len(result.Requirements))
	}
	statuses := make(map[string]string, len(result.Requirements))
	for _, requirement := range result.Requirements {
		statuses[requirement.RequirementID] = requirement.Status
	}
	if statuses["REQ-REVIEW-01"] != "REVIEW_REQUIRED" {
		t.Fatalf("manual review status = %s, want REVIEW_REQUIRED; result=%+v", statuses["REQ-REVIEW-01"], result.Requirements[0])
	}
	if statuses["REQ-REVIEW-02"] != "PASS" {
		t.Fatalf("deterministic Go check should pass independently; statuses=%v", statuses)
	}
	if result.Counts["REVIEW_REQUIRED"] != 1 || result.Counts["UNSUPPORTED"] != 0 {
		t.Fatalf("manual review should be counted separately from unsupported: %v", result.Counts)
	}
	if result.CoverageStatus != "PARTIAL" {
		t.Fatalf("manual review must remain outside automatic coverage; got %s", result.CoverageStatus)
	}
	if result.Verdict != "INDETERMINATE" {
		t.Fatalf("a human-review requirement must prevent an overall PASS, got %s", result.Verdict)
	}
	if result.LLMCalls != 0 {
		t.Fatalf("manual-review verifier unexpectedly made runtime LLM calls: %d", result.LLMCalls)
	}
}

func TestManualReviewOnlyCatalogIsIndeterminateNotUnsupported(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: "1",
		ADRs: []ADR{{
			ID: "ADR-REVIEW-ONLY",
			Title: "Business semantic constraint",
			Version: 1,
			Status: "accepted",
			Requirements: []Requirement{{
				ID: "REQ-REVIEW-ONLY-01",
				Title: "A reviewer must confirm the exception is justified",
				Severity: "warning",
				Verifier: Verifier{Type: "manual-review"},
			}},
		}},
	}
	result := AssessWithOptions(catalog, nil, AssessmentOptions{EvidenceMode: EvidenceModeRepository})
	if result.RequirementCount != 1 || result.Requirements[0].Status != "REVIEW_REQUIRED" {
		t.Fatalf("manual-only requirement should remain explicitly review-required: %+v", result)
	}
	if result.Counts["REVIEW_REQUIRED"] != 1 || result.CoverageStatus != "NONE" || result.Verdict != "INDETERMINATE" {
		t.Fatalf("manual-only catalog must not imply automated coverage: %+v", result)
	}
}

