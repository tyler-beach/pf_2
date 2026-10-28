package orchestrate

import (
	"context"
	"fmt"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/prscan"
)

// FindingConverter maps a deterministic assessment into PR-agent surfaces
// (reviewdog rdjson + Security SARIF). Trust boundary: PR/issue bodies and
// head-tree contents are untrusted inputs; conversion never executes them and
// never elevates N/A/PASS into inline or Security findings.
type FindingConverter interface {
	Convert(ctx context.Context, result assessment.Result, opt prscan.ConvertOptions) (prscan.ConvertResult, error)
}

// DefaultFindingConverter wraps prscan.ConvertAssessment with observability and
// an LLM-taint gate (PR agents must only publish deterministic assessments).
type DefaultFindingConverter struct {
	Observer Observer
}

func NewDefaultFindingConverter() DefaultFindingConverter {
	return DefaultFindingConverter{Observer: NopObserver{}}
}

func (c DefaultFindingConverter) Convert(ctx context.Context, result assessment.Result, opt prscan.ConvertOptions) (prscan.ConvertResult, error) {
	var out prscan.ConvertResult
	attrs := map[string]any{
		"requirement_count": result.RequirementCount,
		"verdict":           result.Verdict,
		"llm_calls":         result.LLMCalls,
		"max_inline":        opt.MaxInline,
		"security_uncapped": opt.SecurityUncapped,
	}
	err := traceStage(ctx, c.Observer, StagePRScan, SeamDeterministic, attrs, func() error {
		if result.LLMCalls != 0 {
			return fmt.Errorf("orchestrate prscan: refusing assessment with %d runtime LLM call(s); PR feedback is a deterministic trust boundary", result.LLMCalls)
		}
		out = prscan.ConvertAssessment(result, opt)
		attrs["posted"] = out.Summary.Posted
		attrs["capped_from"] = out.Summary.CappedFrom
		return nil
	})
	return out, err
}
