package orchestrate

import (
	"context"

	"github.com/tyler-beach/pf_2/internal/plan"
)

// PlanExecutor runs a verified, source-bound analyzer manifest with caching and
// classified retries. It is a deterministic seam (zero LLM calls).
type PlanExecutor interface {
	Execute(ctx context.Context, root string, manifest plan.Manifest, runner plan.TaskRunner, cache plan.ResultCache) (plan.RunResult, error)
}

// DefaultPlanExecutor adapts plan.ExecuteManifest with stage observability.
type DefaultPlanExecutor struct {
	Observer Observer
}

func NewDefaultPlanExecutor() DefaultPlanExecutor {
	return DefaultPlanExecutor{Observer: NopObserver{}}
}

func (e DefaultPlanExecutor) Execute(ctx context.Context, root string, manifest plan.Manifest, runner plan.TaskRunner, cache plan.ResultCache) (plan.RunResult, error) {
	var out plan.RunResult
	attrs := map[string]any{
		"manifest_sha256": manifest.ManifestSHA256,
		"input_sha256":    manifest.InputSHA256,
		"task_count":      len(manifest.Tasks),
		"max_concurrency": manifest.MaxConcurrency,
	}
	err := traceStageKey(ctx, e.Observer, StageExecute, SeamDeterministic, manifest.ManifestSHA256, attrs, func() error {
		result, runErr := plan.ExecuteManifest(ctx, root, manifest, runner, cache)
		out = result
		attrs["run_status"] = result.Status
		attrs["completed_tasks"] = countCompleteTasks(result)
		return runErr
	})
	return out, err
}

func countCompleteTasks(result plan.RunResult) int {
	n := 0
	for _, t := range result.Tasks {
		if t.Status == plan.TaskStatusComplete {
			n++
		}
	}
	return n
}
