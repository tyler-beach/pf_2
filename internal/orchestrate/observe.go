package orchestrate

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// Pipeline stages for Pathfinder's agentic architecture.
// Deterministic runtime stages never invoke an LLM; Assist is batch/offline only.
const (
	StageExtract  = "extract"
	StageAssist   = "assist"
	StageAssess   = "assess"
	StageAnalyze  = "analyzers"
	StagePlan     = "plan"
	StageExecute  = "execute"
	StagePRScan   = "prscan"
	StageWorkshop = "workshop"
	StageReview   = "review"
	StageIngest   = "ingest"
)

// Seams document which stages may call an LLM vs must stay deterministic.
const (
	SeamDeterministic  = "deterministic"
	SeamLLMAssist      = "llm_assist_batch"
	SeamLLMExtractBatch = "llm_extract_batch" // tech_spec/mixed scaffolding only; propose ≠ approve
)

// StageEvent is a structured observability record for orchestration boundaries.
// It is intentionally small so CLI, CI, and future workers share one schema.
type StageEvent struct {
	Stage          string         `json:"stage"`
	Seam           string         `json:"seam"`
	Status         string         `json:"status"` // started|ok|error
	Duration       time.Duration  `json:"duration,omitempty"`
	Attrs          map[string]any `json:"attrs,omitempty"`
	Err            string         `json:"error,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
}

// Observer emits stage lifecycle hooks. Nil-safe via NopObserver.
type Observer interface {
	Stage(ctx context.Context, event StageEvent)
}

// NopObserver discards events.
type NopObserver struct{}

func (NopObserver) Stage(context.Context, StageEvent) {}

// SlogObserver writes JSON-friendly slog records for agent/CI observability.
type SlogObserver struct {
	Logger *slog.Logger
}

func (o SlogObserver) Stage(ctx context.Context, event StageEvent) {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	args := []any{
		"stage", event.Stage,
		"seam", event.Seam,
		"status", event.Status,
	}
	if event.Duration > 0 {
		args = append(args, "duration_ms", event.Duration.Milliseconds())
	}
	if event.IdempotencyKey != "" {
		args = append(args, "idempotency_key", event.IdempotencyKey)
	}
	for k, v := range event.Attrs {
		args = append(args, k, v)
	}
	if event.Err != "" {
		args = append(args, "error", event.Err)
		logger.ErrorContext(ctx, "pathfinder.stage", args...)
		return
	}
	logger.InfoContext(ctx, "pathfinder.stage", args...)
}

// DefaultObserver returns a slog observer writing to stderr (CLI/CI default).
func DefaultObserver() Observer {
	return SlogObserver{Logger: slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))}
}

// observerFrom returns obs or NopObserver.
func observerFrom(obs Observer) Observer {
	if obs == nil {
		return NopObserver{}
	}
	return obs
}

// traceStage records started→ok/error around fn.
func traceStage(ctx context.Context, obs Observer, stage, seam string, attrs map[string]any, fn func() error) error {
	return traceStageKey(ctx, obs, stage, seam, "", attrs, fn)
}

func traceStageKey(ctx context.Context, obs Observer, stage, seam, idempotencyKey string, attrs map[string]any, fn func() error) error {
	obs = observerFrom(obs)
	start := time.Now()
	obs.Stage(ctx, StageEvent{Stage: stage, Seam: seam, Status: "started", Attrs: attrs, IdempotencyKey: idempotencyKey})
	err := fn()
	ev := StageEvent{
		Stage:          stage,
		Seam:           seam,
		Status:         "ok",
		Duration:       time.Since(start),
		Attrs:          attrs,
		IdempotencyKey: idempotencyKey,
	}
	if err != nil {
		ev.Status = "error"
		ev.Err = err.Error()
		obs.Stage(ctx, ev)
		return err
	}
	obs.Stage(ctx, ev)
	return nil
}
