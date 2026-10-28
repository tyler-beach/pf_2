package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

func runIngestADRs(args []string) error {
	fs := flag.NewFlagSet("ingest-adrs", flag.ContinueOnError)
	root := fs.String("root", "adrs", "directory containing ADR Markdown files")
	output := fs.String("json-out", "adr/ingested-adrs.json", "JSON ingestion index output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	docs, err := adringest.ParseDir(*root)
	if err != nil {
		return err
	}
	seen := make(map[string]string, len(docs))
	for _, doc := range docs {
		if previous, ok := seen[doc.ID]; ok {
			return fmt.Errorf("duplicate ADR ID %q in %s and %s", doc.ID, previous, doc.SourcePath)
		}
		seen[doc.ID] = doc.SourcePath
	}
	index := ingestionIndex{
		SchemaVersion: "1",
		SourceRoot:    filepath.ToSlash(filepath.Clean(*root)),
		Documents:     docs,
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ADR ingestion index: %w", err)
	}
	if err := writeFile(*output, data); err != nil {
		return err
	}
	fmt.Printf("Ingested %d ADR document(s) into %s. No obligations were auto-approved.\n", len(docs), *output)
	return nil
}

func runExtractObligations(args []string) error {
	fs := flag.NewFlagSet("extract-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/ingested-adrs.json", "deterministic ADR ingestion index JSON")
	root := fs.String("root", "", "directory containing source ADR Markdown; defaults to source_root from index")
	output := fs.String("json-out", "adr/obligation-candidates.json", "review-only candidate obligation JSON output")
	reportPath := fs.String("report", "", "optional Markdown review queue output")
	minConfidenceReport := fs.Bool("min-confidence-report", false, "print confidence-band summary to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("extract-obligations accepts no positional arguments")
	}
	indexAbs, err := filepath.Abs(*indexPath)
	if err != nil {
		return fmt.Errorf("resolve ingestion index path: %w", err)
	}
	outputAbs, err := filepath.Abs(*output)
	if err != nil {
		return fmt.Errorf("resolve candidate output path: %w", err)
	}
	if filepath.Clean(indexAbs) == filepath.Clean(outputAbs) {
		return fmt.Errorf("candidate output must not overwrite the ADR ingestion index")
	}
	reportAbs := ""
	if *reportPath != "" {
		reportAbs, err = filepath.Abs(*reportPath)
		if err != nil {
			return fmt.Errorf("resolve candidate report path: %w", err)
		}
		if filepath.Clean(reportAbs) == filepath.Clean(indexAbs) || filepath.Clean(reportAbs) == filepath.Clean(outputAbs) {
			return fmt.Errorf("candidate report must not overwrite the ingestion index or candidate JSON output")
		}
	}
	indexBytes, err := os.ReadFile(*indexPath)
	if err != nil {
		return fmt.Errorf("read ADR ingestion index: %w", err)
	}
	var index ingestionIndex
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return fmt.Errorf("decode ADR ingestion index: %w", err)
	}
	if index.SchemaVersion != "1" {
		return fmt.Errorf("unsupported ADR ingestion index schema_version %q", index.SchemaVersion)
	}
	if *root == "" {
		*root = index.SourceRoot
	}
	if strings.TrimSpace(*root) == "" {
		return fmt.Errorf("source root is required via --root or source_root in the index")
	}
	sourceRootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve ADR source root for output safety: %w", err)
	}
	sourceRootReal, err := filepath.EvalSymlinks(sourceRootAbs)
	if err != nil {
		return fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	for _, outputFile := range []string{outputAbs, reportAbs} {
		if outputFile == "" {
			continue
		}
		relative, err := filepath.Rel(sourceRootReal, outputFile)
		if err != nil {
			return fmt.Errorf("validate candidate output containment: %w", err)
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return fmt.Errorf("candidate outputs must remain outside the ADR source root")
		}
	}
	candidates, err := obligations.BuildIndex(index.Documents, *root, index.SourceRoot, indexBytes)
	if err != nil {
		return fmt.Errorf("build review-only obligation candidates: %w", err)
	}
	data, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return fmt.Errorf("encode obligation candidate index: %w", err)
	}
	if *reportPath != "" {
		if err := writeFile(*reportPath, []byte(obligations.MarkdownCandidateReport(candidates))); err != nil {
			return err
		}
	}
	if err := writeFile(*output, data); err != nil {
		return err
	}
	summary := candidates.ConfidenceSummary
	fmt.Printf("Created %d review-required candidate(s) from %d ADR document(s) in %s", candidates.CandidateCount, candidates.DocumentCount, *output)
	if *reportPath != "" {
		fmt.Printf("; Markdown queue: %s", *reportPath)
	}
	fmt.Printf(". No candidate was approved or made enforceable.\n")
	if *minConfidenceReport {
		fmt.Printf("Confidence summary: high=%d medium=%d low=%d (low band is eligible for batch LLM assist)\n",
			summary.High, summary.Medium, summary.Low)
	}
	return nil
}
