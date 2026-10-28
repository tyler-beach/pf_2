package orchestrate

import (
	"context"
	"fmt"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

// ExtractLLMOrchestrator runs kind-gated batch LLM scaffolding on top of
// deterministic extract. Propose ≠ approve: LLM rows stay pending_review.
type ExtractLLMOrchestrator interface {
	Build(ctx context.Context, documents []adringest.Document, sourceRootPath, sourceRootLabel string, sourceIndexRaw []byte, enable bool, opts obligations.ExtractLLMOptions) (obligations.Index, error)
}

// DefaultExtractLLMOrchestrator wraps BuildIndexWithOptionalLLM with HITL asserts.
type DefaultExtractLLMOrchestrator struct {
	Observer Observer
}

func NewDefaultExtractLLMOrchestrator() DefaultExtractLLMOrchestrator {
	return DefaultExtractLLMOrchestrator{Observer: NopObserver{}}
}

func (o DefaultExtractLLMOrchestrator) Build(ctx context.Context, documents []adringest.Document, sourceRootPath, sourceRootLabel string, sourceIndexRaw []byte, enable bool, opts obligations.ExtractLLMOptions) (obligations.Index, error) {
	var out obligations.Index
	attrs := map[string]any{
		"enable":          enable,
		"submission_kind": opts.SubmissionKind,
		"force":           opts.Force,
		"document_count":  len(documents),
	}
	if opts.Provider != nil {
		attrs["provider"] = opts.Provider.Name()
		attrs["model"] = opts.Provider.Model()
	}
	seam := SeamDeterministic
	if enable {
		seam = SeamLLMExtractBatch
	}
	err := traceStage(ctx, o.Observer, StageExtract, seam, attrs, func() error {
		result, buildErr := obligations.BuildIndexWithOptionalLLM(ctx, documents, sourceRootPath, sourceRootLabel, sourceIndexRaw, enable, opts)
		if buildErr != nil {
			return buildErr
		}
		if err := obligations.AssertExtractLLMHITL(result); err != nil {
			return err
		}
		if err := assertExtractLLMHITL(result); err != nil {
			return err
		}
		out = result
		attrs["candidate_count"] = result.CandidateCount
		attrs["llm_candidate_count"] = result.LLMExtract.LLMCandidateCount
		attrs["runtime_llm_calls"] = result.LLMExtract.RuntimeLLMCalls
		attrs["prompt_tokens"] = result.LLMExtract.PromptTokens
		attrs["completion_tokens"] = result.LLMExtract.CompletionTokens
		return nil
	})
	return out, err
}

// assertExtractLLMHITL is defense-in-depth on top of obligations.AssertExtractLLMHITL.
func assertExtractLLMHITL(index obligations.Index) error {
	for _, c := range index.Candidates {
		if c.ExtractionMethod != obligations.ExtractionMethodLLM {
			continue
		}
		if c.Executable {
			return fmt.Errorf("orchestrate HITL: extract-llm candidate %s marked executable; must never auto-approve", c.ID)
		}
		if c.ApprovalStatus != "" && c.ApprovalStatus != obligations.DecisionPending {
			return fmt.Errorf("orchestrate HITL: extract-llm candidate %s approval_status=%q; must remain pending_review", c.ID, c.ApprovalStatus)
		}
	}
	return nil
}
