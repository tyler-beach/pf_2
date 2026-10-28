package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/orchestrate"
)

func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	path := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := assessment.LoadCatalog(*path)
	if err != nil {
		return err
	}
	n := 0
	for _, a := range c.ADRs {
		n += len(a.Requirements)
	}
	fmt.Printf("Catalog valid: %d ADR(s), %d requirement(s).\n", len(c.ADRs), n)
	return nil
}

func runAssess(args []string, repoMode bool) error {
	return runAssessCtx(context.Background(), args, repoMode)
}

func runAssessCtx(ctx context.Context, args []string, repoMode bool) error {
	fs := flag.NewFlagSet("assess", flag.ContinueOnError)
	input := fs.String("input", "", "Markdown issue body")
	root := fs.String("root", ".", "repository root")
	catalogPath := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	reportPath := fs.String("report", "report.md", "Markdown report output")
	jsonPath := fs.String("json-out", "assessment.json", "JSON evidence output")
	sarifPath := fs.String("sarif-out", "results.sarif", "SARIF output")
	analyzerEvidencePath := fs.String("analyzer-evidence", "", "comma-separated normalized analyzer evidence JSON paths (repository assessment only)")
	analyzerSARIFPath := fs.String("analyzer-sarif", "", "comma-separated raw SARIF paths paired 1:1 with --analyzer-evidence")
	issueSurface := fs.Bool("issue-surface", false, "GitHub-issue/sandbox semantics: manual-review and missing analyzer evidence become NOT_APPLICABLE")
	submissionKindFlag := fs.String("submission-kind", "", "input shape: tech_spec (ADR/prose) or code_snippet (source/IaC); empty auto-detects")
	scope := fs.String("scope", "", "comma-separated repository-relative path prefixes (shared with scanner --scope / PR changed paths); empty = full tree")
	assessAll := fs.Bool("assess-all", false, "evaluate the full catalog without applicability filtering (corpus CI)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	explicitKind, err := assessment.ParseSubmissionKind(*submissionKindFlag)
	if err != nil {
		return err
	}
	var files []assessment.SourceFile
	var issueMarkdown string
	if repoMode {
		files, err = assessment.LoadSourceFiles(*root)
	} else {
		if *input == "" {
			return fmt.Errorf("--input is required for issue assessment")
		}
		var data []byte
		data, err = os.ReadFile(*input)
		if err == nil {
			issueMarkdown = string(data)
			files = assessment.ParseIssueMarkdown(issueMarkdown)
		}
	}
	if err != nil {
		return fmt.Errorf("load assessment sources: %w", err)
	}
	mode := assessment.EvidenceModeSubmission
	hint := assessment.SubmissionKindUnspecified
	if repoMode {
		mode = assessment.EvidenceModeRepository
		hint = assessment.DefaultRepositorySubmissionKind()
	} else if issueMarkdown != "" {
		hint = assessment.ClassifyIssueSubmissionKind(issueMarkdown)
	}
	analyzerResults, err := loadVerifiedAnalyzerEvidence(*root, repoMode, *analyzerEvidencePath, *analyzerSARIFPath)
	if err != nil {
		return err
	}
	// AssessmentOrchestrator is the production DI seam (Uber Go: accept interfaces
	// at edges). Deterministic: zero LLM calls; Observer emits stage JSON when enabled.
	orch := orchestrate.DefaultOrchestrator{
		Catalogs: orchestrate.CatalogFileProvider{},
		Evaluate: orchestrate.DefaultEvaluator{},
		Observer: cliObserver(),
	}
	result, err := orch.Assess(ctx, *catalogPath, files, assessment.AssessmentOptions{
		EvidenceMode:    mode,
		AnalyzerResults: analyzerResults,
		IssueSurface:    *issueSurface,
		SubmissionKind:  assessment.ResolveSubmissionKind(explicitKind, hint),
		ScopePrefixes:   splitScopePrefixes(*scope),
		AssessAll:       *assessAll,
	})
	if err != nil {
		return fmt.Errorf("assess: %w", err)
	}
	if err := writeFile(*reportPath, []byte(assessment.MarkdownReport(result))); err != nil {
		return err
	}
	j, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFile(*jsonPath, j); err != nil {
		return err
	}
	s, err := assessment.SARIFReport(result)
	if err != nil {
		return err
	}
	if err := writeFile(*sarifPath, s); err != nil {
		return err
	}
	fmt.Printf("Verdict: %s; requirements: %d; source files: %d; LLM calls: %d\n", result.Verdict, result.RequirementCount, result.SourceFileCount, result.LLMCalls)
	fmt.Print(assessment.MarkdownReport(result))
	return nil
}

// runAssertIssueVerdict makes GitHub issue regressions executable rather than
// merely descriptive. Add exactly one [expected:PASS], [expected:FAIL], or
// [expected:INDETERMINATE] marker to the issue title to assert its assessment.
func runAssertIssueVerdict(args []string) error {
	fs := flag.NewFlagSet("assert-issue-verdict", flag.ContinueOnError)
	title := fs.String("title", "", "GitHub issue title")
	assessmentPath := fs.String("assessment", "assessment.json", "assessment JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return fmt.Errorf("--title is required")
	}
	data, err := os.ReadFile(*assessmentPath)
	if err != nil {
		return fmt.Errorf("read assessment output: %w", err)
	}
	var result assessment.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode assessment output: %w", err)
	}
	matched, err := assessment.CheckExpectedVerdict(*title, result)
	if err != nil {
		return err
	}
	if !matched {
		fmt.Println("No expected-verdict marker in issue title; assertion skipped.")
		return nil
	}
	fmt.Printf("Issue regression matched: verdict=%s coverage=%s\n", result.Verdict, result.CoverageStatus)
	return nil
}
