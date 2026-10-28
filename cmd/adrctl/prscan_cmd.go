package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/prscan"
)

func runParsePRURL(args []string) error {
	fs := flag.NewFlagSet("parse-pr-url", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: adrctl parse-pr-url <url>")
	}
	ref, err := prscan.ParsePRURL(fs.Arg(0))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(ref)
}

func runParsePRScanIssue(args []string) error {
	fs := flag.NewFlagSet("parse-pr-scan-issue", flag.ContinueOnError)
	path := fs.String("body", "", "issue body file (or stdin if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var raw []byte
	var err error
	if *path == "" || *path == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(*path)
	}
	if err != nil {
		return err
	}
	req, err := prscan.ParseIssueBody(string(raw))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(req)
}

func runAssessmentToRDJSON(args []string) error {
	fs := flag.NewFlagSet("assessment-to-rdjson", flag.ContinueOnError)
	assessmentPath := fs.String("assessment", "assessment.json", "assessment JSON input")
	rdjsonPath := fs.String("rdjson-out", "pathfinder.rdjson", "rdjson output path")
	sarifPath := fs.String("sarif-out", "pathfinder-security.sarif", "GitHub code-scanning SARIF output")
	summaryPath := fs.String("summary-out", "pathfinder-pr-summary.md", "markdown summary output")
	maxInline := fs.Int("max-inline", 25, "max high-signal inline diagnostics")
	securityUncapped := fs.Bool("security-uncapped", false, "include all deviations in Security SARIF (still no N/A)")
	docsBase := fs.String("docs-base", prscan.DefaultDocsBase, "docs link embedded in Next lines")
	prURL := fs.String("pr-url", "", "target PR URL for summary")
	headSHA := fs.String("head-sha", "", "PR head SHA for summary / dedupe marker")
	issueURL := fs.String("issue-url", "", "originating scan issue URL")
	marker := fs.String("marker", "", "dedupe marker (defaults to head SHA)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*assessmentPath)
	if err != nil {
		return fmt.Errorf("read assessment: %w", err)
	}
	var result assessment.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("decode assessment: %w", err)
	}
	mark := *marker
	if mark == "" {
		mark = *headSHA
	}
	conv := prscan.ConvertAssessment(result, prscan.ConvertOptions{
		MaxInline:        *maxInline,
		SecurityUncapped: *securityUncapped,
		DocsBase:         *docsBase,
		PRURL:            *prURL,
		HeadSHA:          *headSHA,
		IssueURL:         *issueURL,
		Marker:           mark,
	})
	raw, err := prscan.MarshalRDJSON(conv.RDJSON)
	if err != nil {
		return err
	}
	if err := writeFile(*rdjsonPath, append(raw, '\n')); err != nil {
		return err
	}
	sarifRaw, err := prscan.MarshalSecuritySARIF(conv.SecuritySARIF)
	if err != nil {
		return err
	}
	if err := writeFile(*sarifPath, append(sarifRaw, '\n')); err != nil {
		return err
	}
	summary := prscan.FormatSummary(conv.Summary)
	if err := writeFile(*summaryPath, []byte(summary)); err != nil {
		return err
	}
	sarifResults := 0
	if len(conv.SecuritySARIF.Runs) > 0 {
		sarifResults = len(conv.SecuritySARIF.Runs[0].Results)
	}
	fmt.Printf("rdjson diagnostics=%d security_sarif_results=%d capped_from=%d summary=%s rdjson=%s sarif=%s\n",
		conv.Summary.Posted, sarifResults, conv.Summary.CappedFrom, *summaryPath, *rdjsonPath, *sarifPath)
	return nil
}
