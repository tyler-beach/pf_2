// Package orchestrate defines thin orchestration interfaces around existing
// Pathfinder packages. Implementations wrap current behavior without changing
// assessment semantics (scaling-adr-design Step A).
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
type AssessmentOrchestrator interface {
	Assess(catalogPath string, files []assessment.SourceFile, options assessment.AssessmentOptions) (assessment.Result, error)
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
}

func NewDefaultOrchestrator() DefaultOrchestrator {
	return DefaultOrchestrator{Catalogs: CatalogFileProvider{}, Evaluate: DefaultEvaluator{}}
}

func (o DefaultOrchestrator) Assess(catalogPath string, files []assessment.SourceFile, options assessment.AssessmentOptions) (assessment.Result, error) {
	if o.Catalogs == nil {
		return assessment.Result{}, fmt.Errorf("orchestrate: CatalogProvider is required")
	}
	if o.Evaluate == nil {
		return assessment.Result{}, fmt.Errorf("orchestrate: PatternEvaluator is required")
	}
	catalog, err := o.Catalogs.Load(catalogPath)
	if err != nil {
		return assessment.Result{}, fmt.Errorf("load requirements catalog: %w", err)
	}
	return o.Evaluate.Evaluate(catalog, files, options), nil
}
