package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

func runAssessObligations(args []string) error {
	fs := flag.NewFlagSet("assess-obligations", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "source-cited candidate index")
	ingestionIndexPath := fs.String("ingestion-index", "adr/ingested-adrs.json", "source ADR ingestion index used to reproduce candidates")
	catalogPath := fs.String("requirements", "adr/requirements.json", "approved requirements catalog")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "append-only review registry")
	assessmentPath := fs.String("assessment", "assessment.json", "JSON result from assess-repo")
	root := fs.String("root", ".", "current repository root for stale-evidence validation")
	jsonPath := fs.String("json-out", "obligation-assessment.json", "per-obligation JSON report output")
	markdownPath := fs.String("report", "obligation-assessment.md", "per-obligation Markdown report output")
	enforceBlocking := fs.Bool("enforce-blocking", false, "return an error unless every approved blocking obligation passes with fresh evidence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("assess-obligations accepts no positional arguments")
	}
	inputs := []string{*indexPath, *catalogPath, *registryPath, *assessmentPath}
	outputs := []string{*jsonPath, *markdownPath}
	seen := map[string]string{}
	for _, input := range inputs {
		absolute, err := filepath.Abs(input)
		if err != nil {
			return fmt.Errorf("resolve input path %q: %w", input, err)
		}
		seen[filepath.Clean(absolute)] = "input"
	}
	for _, output := range outputs {
		absolute, err := filepath.Abs(output)
		if err != nil {
			return fmt.Errorf("resolve output path %q: %w", output, err)
		}
		key := filepath.Clean(absolute)
		if prior, exists := seen[key]; exists {
			return fmt.Errorf("output path %q collides with an existing %s path", output, prior)
		}
		seen[key] = "output"
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	ingestionBytes, err := os.ReadFile(*ingestionIndexPath)
	if err != nil {
		return fmt.Errorf("read ADR ingestion index for candidate freshness: %w", err)
	}
	var sourceIndex ingestionIndex
	if err := json.Unmarshal(ingestionBytes, &sourceIndex); err != nil {
		return fmt.Errorf("decode ADR ingestion index for candidate freshness: %w", err)
	}
	if sourceIndex.SchemaVersion != "1" {
		return fmt.Errorf("unsupported ADR ingestion index schema_version %q", sourceIndex.SchemaVersion)
	}
	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	sourceRootPath := sourceIndex.SourceRoot
	if !filepath.IsAbs(sourceRootPath) {
		sourceRootPath = filepath.Join(rootReal, sourceRootPath)
	}
	sourceRootAbs, err := filepath.Abs(sourceRootPath)
	if err != nil {
		return fmt.Errorf("resolve ADR source root: %w", err)
	}
	sourceRootReal, err := filepath.EvalSymlinks(sourceRootAbs)
	if err != nil {
		return fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	relativeSourceRoot, err := filepath.Rel(rootReal, sourceRootReal)
	if err != nil {
		return fmt.Errorf("validate ADR source root containment: %w", err)
	}
	if relativeSourceRoot == ".." || strings.HasPrefix(relativeSourceRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeSourceRoot) {
		return fmt.Errorf("ADR source root must remain inside the current repository root")
	}
	rebuiltIndex, err := obligations.BuildIndex(sourceIndex.Documents, sourceRootReal, sourceIndex.SourceRoot, ingestionBytes)
	if err != nil {
		return fmt.Errorf("rebuild candidate index from current ADR sources: %w", err)
	}
	expectedIndex, err := json.Marshal(rebuiltIndex)
	if err != nil {
		return fmt.Errorf("encode rebuilt candidate index: %w", err)
	}
	providedIndex, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("encode provided candidate index: %w", err)
	}
	if !bytes.Equal(expectedIndex, providedIndex) {
		return fmt.Errorf("candidate index does not match current ADR sources and ingestion index; rerun ingest-adrs and extract-obligations")
	}
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*assessmentPath)
	if err != nil {
		return fmt.Errorf("read repository assessment: %w", err)
	}
	var evidence assessment.Result
	if err := json.Unmarshal(data, &evidence); err != nil {
		return fmt.Errorf("decode repository assessment: %w", err)
	}
	currentFiles, err := assessment.LoadSourceFiles(*root)
	if err != nil {
		return fmt.Errorf("load current repository source inventory: %w", err)
	}
	currentSourceSnapshot := assessment.HashSourceFiles(currentFiles)
	currentRepositorySnapshot, err := analyzer.HashInputTree(*root)
	if err != nil {
		return fmt.Errorf("hash current repository snapshot: %w", err)
	}
	report, err := obligations.AssessCandidates(index, catalog, registry, evidence, currentSourceSnapshot, currentRepositorySnapshot)
	if err != nil {
		return fmt.Errorf("assess obligation candidates: %w", err)
	}
	indexBytes, err := os.ReadFile(*indexPath)
	if err != nil {
		return fmt.Errorf("read candidate index for provenance: %w", err)
	}
	registryBytes, err := os.ReadFile(*registryPath)
	if err != nil {
		return fmt.Errorf("read review registry for provenance: %w", err)
	}
	indexDigest := sha256.Sum256(indexBytes)
	registryDigest := sha256.Sum256(registryBytes)
	assessmentDigest := sha256.Sum256(data)
	report.CandidateIndexArtifactSHA256 = hex.EncodeToString(indexDigest[:])
	report.ReviewRegistryArtifactSHA256 = hex.EncodeToString(registryDigest[:])
	report.AssessmentArtifactSHA256 = hex.EncodeToString(assessmentDigest[:])
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode obligation assessment: %w", err)
	}
	if err := writeFile(*jsonPath, encoded); err != nil {
		return err
	}
	if err := writeFile(*markdownPath, []byte(obligations.MarkdownAssessmentReport(report))); err != nil {
		return err
	}
	fmt.Printf("Obligation assessment: %d candidate(s); verdict=%s; PASS=%d FAIL=%d INDETERMINATE=%d NOT_APPLICABLE=%d; JSON=%s; report=%s\n",
		report.CandidateCount, report.ObligationVerdict, report.Counts["PASS"], report.Counts["FAIL"], report.Counts["INDETERMINATE"],
		report.Counts["NOT_APPLICABLE"], *jsonPath, *markdownPath)
	if *enforceBlocking {
		if err := validateBlockingGate(report); err != nil {
			return fmt.Errorf("%w; see %s", err, *markdownPath)
		}
	}
	return nil
}

// runSpecificationCheck orchestrates the deterministic source-to-evidence
// lifecycle. It always regenerates ingestion/candidate artifacts, so source
// drift cannot be hidden by a stale checked-in candidate index.
func runSpecificationCheck(args []string) error {
	fs := flag.NewFlagSet("specification-check", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	var sourceRoot string
	fs.StringVar(&sourceRoot, "source-root", "adrs", "repository-relative directory containing ADR or plain-English specification Markdown")
	fs.StringVar(&sourceRoot, "adr-root", "adrs", "legacy alias for --source-root")
	requirements := fs.String("requirements", "adr/requirements.json", "accepted requirements catalog JSON")
	registry := fs.String("registry", "adr/obligation-reviews.json", "append-only obligation review registry JSON")
	analyzerEvidence := fs.String("analyzer-evidence", "", "optional normalized analyzer scan evidence JSON")
	analyzerSARIF := fs.String("analyzer-sarif", "", "raw SARIF paired with normalized analyzer evidence")
	reportOnly := fs.Bool("report-only", false, "write assessment artifacts without enforcing blocking obligations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("specification-check accepts no positional arguments")
	}
	if (*analyzerEvidence == "") != (*analyzerSARIF == "") {
		return fmt.Errorf("--analyzer-evidence and --analyzer-sarif must be provided together")
	}

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolveInput := func(raw string) string {
		if filepath.IsAbs(raw) {
			return filepath.Clean(raw)
		}
		return filepath.Join(rootReal, raw)
	}
	adrRootAbs, err := filepath.Abs(resolveInput(sourceRoot))
	if err != nil {
		return fmt.Errorf("resolve ADR source root: %w", err)
	}
	adrRootReal, err := filepath.EvalSymlinks(adrRootAbs)
	if err != nil {
		return fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	relativeADRRoot, err := filepath.Rel(rootReal, adrRootReal)
	if err != nil {
		return fmt.Errorf("validate ADR source root containment: %w", err)
	}
	if relativeADRRoot == ".." || strings.HasPrefix(relativeADRRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeADRRoot) {
		return fmt.Errorf("ADR source root must remain inside repository root")
	}
	artifactDir := filepath.Join(rootReal, "adr")
	if resolved, resolveErr := filepath.EvalSymlinks(artifactDir); resolveErr == nil {
		artifactDir = resolved
	} else if !os.IsNotExist(resolveErr) {
		return fmt.Errorf("resolve generated artifact directory: %w", resolveErr)
	}
	relativeArtifactDir, err := filepath.Rel(rootReal, artifactDir)
	if err != nil || relativeArtifactDir == ".." || strings.HasPrefix(relativeArtifactDir, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeArtifactDir) {
		return fmt.Errorf("generated artifact directory must remain inside repository root")
	}
	relArtifacts, err := filepath.Rel(adrRootReal, artifactDir)
	if err != nil {
		return fmt.Errorf("validate generated artifact directory: %w", err)
	}
	if relArtifacts == "." || (relArtifacts != ".." && !strings.HasPrefix(relArtifacts, ".."+string(filepath.Separator))) {
		return fmt.Errorf("ADR source root must not contain the generated artifact directory %q", artifactDir)
	}

	catalogPath, registryPath := resolveInput(*requirements), resolveInput(*registry)
	ingestionPath := filepath.Join(artifactDir, "ingested-adrs.json")
	candidatePath := filepath.Join(artifactDir, "obligation-candidates.json")
	candidateReportPath := filepath.Join(artifactDir, "obligation-candidates.md")
	assessmentPath := filepath.Join(rootReal, "assessment.json")
	repositoryReportPath := filepath.Join(rootReal, "report.md")
	sarifPath := filepath.Join(rootReal, "results.sarif")
	obligationReportPath := filepath.Join(rootReal, "obligation-assessment.json")
	obligationMarkdownPath := filepath.Join(rootReal, "obligation-assessment.md")
	outputs := []string{ingestionPath, candidatePath, candidateReportPath, assessmentPath, repositoryReportPath, sarifPath, obligationReportPath, obligationMarkdownPath}
	canonicalOutputs := make([]string, len(outputs))
	outputOwners := make(map[string]string, len(outputs))
	for i, output := range outputs {
		canonicalOutput := canonicalPathForCollision(output)
		relativeOutput, err := filepath.Rel(rootReal, canonicalOutput)
		if err != nil || relativeOutput == ".." ||
			strings.HasPrefix(relativeOutput, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(relativeOutput) {
			return fmt.Errorf("generated output %q resolves outside repository root", output)
		}
		if previous, exists := outputOwners[canonicalOutput]; exists {
			return fmt.Errorf("generated output paths %q and %q resolve to the same target", previous, output)
		}
		outputOwners[canonicalOutput] = output
		canonicalOutputs[i] = canonicalOutput
	}

	inputs := []string{catalogPath, registryPath}
	if *analyzerEvidence != "" {
		inputs = append(inputs, resolveInput(*analyzerEvidence), resolveInput(*analyzerSARIF))
	}
	for _, input := range inputs {
		canonicalInput := canonicalPathForCollision(input)
		for i, output := range outputs {
			if canonicalInput == canonicalOutputs[i] {
				return fmt.Errorf("input path %q collides with generated output %q", input, output)
			}
		}
	}

	if err := runIngestADRs([]string{"--root", adrRootReal, "--json-out", ingestionPath}); err != nil {
		return fmt.Errorf("ingest ADR sources: %w", err)
	}
	if err := runExtractObligations([]string{"--index", ingestionPath, "--root", adrRootReal, "--json-out", candidatePath, "--report", candidateReportPath}); err != nil {
		return fmt.Errorf("extract obligation candidates: %w", err)
	}
	assessArgs := []string{"--root", rootReal, "--requirements", catalogPath, "--report", repositoryReportPath, "--json-out", assessmentPath, "--sarif-out", sarifPath}
	if *analyzerEvidence != "" {
		assessArgs = append(assessArgs, "--analyzer-evidence", resolveInput(*analyzerEvidence), "--analyzer-sarif", resolveInput(*analyzerSARIF))
	}
	if err := runAssess(assessArgs, true); err != nil {
		return fmt.Errorf("assess repository: %w", err)
	}
	joinArgs := []string{
		"--root", rootReal, "--index", candidatePath, "--ingestion-index", ingestionPath,
		"--requirements", catalogPath, "--registry", registryPath, "--assessment", assessmentPath,
		"--json-out", obligationReportPath, "--report", obligationMarkdownPath,
	}
	if !*reportOnly {
		joinArgs = append(joinArgs, "--enforce-blocking")
	}
	if err := runAssessObligations(joinArgs); err != nil {
		return fmt.Errorf("specification check failed: %w", err)
	}
	return nil
}

func validateBlockingGate(report obligations.AssessmentReport) error {
	if report.BindingCoverage.OrphanedCount > 0 {
		return fmt.Errorf("%d orphaned review history record(s) must be explicitly superseded before enforcing blocking obligations", report.BindingCoverage.OrphanedCount)
	}
	if report.BindingCoverage.StaleCount > 0 {
		return fmt.Errorf("%d stale candidate review or verifier binding snapshot(s) must be re-reviewed before enforcing blocking obligations", report.BindingCoverage.StaleCount)
	}
	for _, candidate := range report.Candidates {
		// An unreviewed or source-stale candidate may represent a newly
		// introduced blocking obligation. Do not let absence of an approval
		// silently bypass the enforcement gate.
		if candidate.Decision == obligations.DecisionPending {
			return fmt.Errorf("obligation candidate %s remains pending review (status=%s); review or explicitly reject it before enforcing blocking obligations", candidate.CandidateID, candidate.Status)
		}
		if candidate.Decision == obligations.DecisionApproved && candidate.EnforcementMode == obligations.EnforcementBlocking && candidate.Status != "PASS" {
			return fmt.Errorf("blocking obligation %s did not pass (status=%s, evidence=%s)", candidate.CandidateID, candidate.Status, candidate.EvidenceStatus)
		}
	}
	return nil
}
