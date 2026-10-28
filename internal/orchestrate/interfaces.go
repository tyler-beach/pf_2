// Package orchestrate defines thin orchestration interfaces around existing
// Pathfinder packages. Implementations wrap current behavior without changing
// assessment semantics (scaling-adr-design Step A).
//
// Agentic architecture contracts (tool/agent boundaries):
//
//	ingest → extract (deterministic ± optional LLM scaffolding) → assist (LLM batch, HITL) → review (human)
//	plan → execute/analyzers (deterministic) → assess (deterministic) → prscan / workshop
//
// Runtime assess/PR gates make zero LLM calls. Assist / extract-llm never auto-approve.
package orchestrate

import (
	"context"
	"fmt"

	"github.com/tyler-beach/pf_2/internal/analyzer"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/evidence"
	"github.com/tyler-beach/pf_2/internal/plan"
)

// Analyzer runs a source-bound plan task and returns raw-normalized scan output.
type Analyzer interface {
	Run(ctx context.Context, root string, task plan.Task) (analyzer.ScanResult, error)
}

// Normalizer converts vendor scan output into the canonical evidence envelope.
type Normalizer interface {
	Normalize(scan analyzer.ScanResult, runID string) evidence.Bundle
}

// PatternEvaluator evaluates one requirement against source files and evidence.
type PatternEvaluator interface {
	Evaluate(catalog assessment.Catalog, files []assessment.SourceFile, options assessment.AssessmentOptions) assessment.Result
}

// CatalogProvider loads and validates the accepted requirements catalog.
type CatalogProvider interface {
	Load(path string) (assessment.Catalog, error)
}

// AssessmentOrchestrator coordinates catalog load, evidence attachment, and evaluation.
// Deterministic seam: implementations must keep Result.LLMCalls at zero.
type AssessmentOrchestrator interface {
	Assess(ctx context.Context, catalogPath string, files []assessment.SourceFile, options assessment.AssessmentOptions) (assessment.Result, error)
}

// OpenGrepAnalyzer adapts plan.OpenGrepTaskRunner to Analyzer.
type OpenGrepAnalyzer struct {
	Runner plan.OpenGrepTaskRunner
}

func (a OpenGrepAnalyzer) Run(ctx context.Context, root string, task plan.Task) (analyzer.ScanResult, error) {
	return a.Runner.Run(ctx, root, task)
}

// ScanNormalizer adapts evidence.FromScanResult.
type ScanNormalizer struct{}

func (ScanNormalizer) Normalize(scan analyzer.ScanResult, runID string) evidence.Bundle {
	return evidence.FromScanResult(scan, runID)
}

// CatalogFileProvider loads catalogs from disk.
type CatalogFileProvider struct{}

func (CatalogFileProvider) Load(path string) (assessment.Catalog, error) {
	return assessment.LoadCatalog(path)
}

// DefaultEvaluator wraps assessment.AssessWithOptions.
type DefaultEvaluator struct{}

func (DefaultEvaluator) Evaluate(catalog assessment.Catalog, files []assessment.SourceFile, options assessment.AssessmentOptions) assessment.Result {
	return assessment.AssessWithOptions(catalog, files, options)
}

// DefaultOrchestrator is the production wiring of CatalogProvider + PatternEvaluator.
type DefaultOrchestrator struct {
	Catalogs CatalogProvider
	Evaluate PatternEvaluator
	Observer Observer
}

func NewDefaultOrchestrator() DefaultOrchestrator {
	// Observer defaults to nop so unit tests stay quiet; CLI wires DefaultObserver.
	return DefaultOrchestrator{
		Catalogs: CatalogFileProvider{},
		Evaluate: DefaultEvaluator{},
		Observer: NopObserver{},
	}
}

func (o DefaultOrchestrator) Assess(ctx context.Context, catalogPath string, files []assessment.SourceFile, options assessment.AssessmentOptions) (assessment.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return assessment.Result{}, err
	}
	if o.Catalogs == nil {
		return assessment.Result{}, fmt.Errorf("orchestrate: CatalogProvider is required")
	}
	if o.Evaluate == nil {
		return assessment.Result{}, fmt.Errorf("orchestrate: PatternEvaluator is required")
	}
	var result assessment.Result
	attrs := map[string]any{
		"catalog_path":      catalogPath,
		"source_file_count": len(files),
		"evidence_mode":     options.EvidenceMode,
		"issue_surface":     options.IssueSurface,
		"analyzer_results":  len(options.AnalyzerResults),
	}
	err := traceStage(ctx, o.Observer, StageAssess, SeamDeterministic, attrs, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		catalog, loadErr := o.Catalogs.Load(catalogPath)
		if loadErr != nil {
			return fmt.Errorf("load requirements catalog: %w", loadErr)
		}
		result = o.Evaluate.Evaluate(catalog, files, options)
		if result.LLMCalls != 0 {
			return fmt.Errorf("orchestrate: assess seam violated — runtime LLM calls = %d (must be 0)", result.LLMCalls)
		}
		attrs["verdict"] = result.Verdict
		attrs["requirement_count"] = result.RequirementCount
		attrs["llm_calls"] = result.LLMCalls
		return nil
	})
	return result, err
}
