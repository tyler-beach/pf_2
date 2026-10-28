package orchestrate

import (
	"context"
	"fmt"

	"github.com/tyler-beach/pf_2/internal/obligations"
)

// AssistOrchestrator is the sole LLM seam. It produces review-only proposals and
// must never mutate the requirements catalog or mark obligations executable.
// Human acceptance is a separate accept-assist-proposal / record-obligation-review
// path (propose ≠ approve).
type AssistOrchestrator interface {
	Assist(ctx context.Context, index obligations.Index, opts obligations.AssistOptions) (obligations.ProposalIndex, error)
}

// DefaultAssistOrchestrator wraps obligations.AssistObligations with HITL
// invariants and structured stage observability (Anthropic HITL / LangGraph
// interrupt-before-commit pattern).
type DefaultAssistOrchestrator struct {
	Observer Observer
}

func NewDefaultAssistOrchestrator() DefaultAssistOrchestrator {
	return DefaultAssistOrchestrator{Observer: NopObserver{}}
}

func (o DefaultAssistOrchestrator) Assist(ctx context.Context, index obligations.Index, opts obligations.AssistOptions) (obligations.ProposalIndex, error) {
	var out obligations.ProposalIndex
	attrs := map[string]any{
		"max_confidence": opts.MaxConfidence,
		"max_candidates": opts.MaxCandidates,
	}
	if opts.Provider != nil {
		attrs["provider"] = opts.Provider.Name()
		attrs["model"] = opts.Provider.Model()
	}
	err := traceStage(ctx, o.Observer, StageAssist, SeamLLMAssist, attrs, func() error {
		result, assistErr := obligations.AssistObligations(ctx, index, opts)
		if assistErr != nil {
			return assistErr
		}
		if err := assertAssistHITL(result); err != nil {
			return err
		}
		out = result
		attrs["proposal_count"] = result.ProposalCount
		attrs["candidate_count"] = result.CandidateCount
		attrs["runtime_llm_calls"] = result.RuntimeLLMCalls
		attrs["prompt_tokens"] = result.PromptTokens
		attrs["completion_tokens"] = result.CompletionTokens
		return nil
	})
	return out, err
}

// assertAssistHITL fails closed if any proposal looks approved or executable.
// Defense-in-depth on top of obligations.AssistObligations invariants.
func assertAssistHITL(index obligations.ProposalIndex) error {
	for _, p := range index.Proposals {
		if p.Executable {
			return fmt.Errorf("orchestrate HITL: proposal %s marked executable; assist must never auto-approve", p.ProposalID)
		}
		if p.ApprovalStatus != "" && p.ApprovalStatus != obligations.DecisionPending {
			return fmt.Errorf("orchestrate HITL: proposal %s approval_status=%q; assist must remain pending_review", p.ProposalID, p.ApprovalStatus)
		}
	}
	return nil
}
