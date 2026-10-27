package workshop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/obligations"
)

const SnapshotSchemaVersion = "workshop-snapshot-v1"
const MetricsSchemaVersion = "workshop-metrics-v1"

// RunIdentity identifies one Pathfinder workflow execution.
type RunIdentity struct {
	RunID        string `json:"run_id"`
	Workflow     string `json:"workflow"`
	GitSHA       string `json:"git_sha"`
	Timestamp    string `json:"timestamp"`
	HTMLURL      string `json:"html_url,omitempty"`
	ProviderMode string `json:"provider_mode,omitempty"`
}

// Metrics is the flat KPI map matching docs/kpi-dictionary.md.
type Metrics struct {
	SchemaVersion string `json:"schema_version"`

	CandidateTotal        int      `json:"candidate_total"`
	ConfidenceHigh        int      `json:"confidence_high"`
	ConfidenceMedium      int      `json:"confidence_medium"`
	ConfidenceLow         int      `json:"confidence_low"`
	ConfidenceShareHigh   *float64 `json:"kpi.confidence.share.high,omitempty"`
	ConfidenceShareMedium *float64 `json:"kpi.confidence.share.medium,omitempty"`
	ConfidenceShareLow    *float64 `json:"kpi.confidence.share.low,omitempty"`
	EscalationRate        float64  `json:"kpi.escalation.rate"`

	HarnessPrecision    *float64 `json:"kpi.harness.precision,omitempty"`
	HarnessRecall       *float64 `json:"kpi.harness.recall,omitempty"`
	HarnessRecallHigh   *float64 `json:"kpi.harness.recall.high,omitempty"`
	HarnessRecallMedium *float64 `json:"kpi.harness.recall.medium,omitempty"`
	HarnessRecallLow    *float64 `json:"kpi.harness.recall.low,omitempty"`

	ReviewBacklog    *int     `json:"kpi.review.backlog,omitempty"`
	PendingHigh      *int     `json:"kpi.review.pending_high,omitempty"`
	PendingMedium    *int     `json:"kpi.review.pending_medium,omitempty"`
	PendingLow       *int     `json:"kpi.review.pending_low,omitempty"`
	AssistAcceptRate *float64 `json:"kpi.review.accept_rate,omitempty"`

	AssessVerdict   string `json:"kpi.assess.verdict,omitempty"`
	AssessCovered   *int   `json:"kpi.assess.covered,omitempty"`
	AssessUnbound   *int   `json:"kpi.assess.unbound,omitempty"`
	AssessStale     *int   `json:"kpi.assess.stale,omitempty"`
	RuntimeLLMCalls *int   `json:"kpi.invariant.llm_calls,omitempty"`

	AssistProposalCount    *int     `json:"kpi.assist.proposal_count,omitempty"`
	AssistPromptTokens     *int     `json:"kpi.assist.prompt_tokens,omitempty"`
	AssistCompletionTokens *int     `json:"kpi.assist.completion_tokens,omitempty"`
	AssistCostUSD          *float64 `json:"kpi.assist.cost_usd,omitempty"`
	AssistProvider         string   `json:"kpi.assist.provider,omitempty"`
	AssistModel            string   `json:"kpi.assist.model,omitempty"`
	PromptfooPassRate      *float64 `json:"kpi.promptfoo.pass_rate,omitempty"`
	PromptfooMode          string   `json:"promptfoo_mode,omitempty"`

	EvidenceGapRate *float64 `json:"kpi.evidence.gap_rate,omitempty"`
	CorpusSize      *int     `json:"kpi.corpus.size,omitempty"`
}

// Latest is the operational rollup for the workshop home page.
type Latest struct {
	SchemaVersion string            `json:"schema_version"`
	Run           RunIdentity       `json:"run"`
	Metrics       Metrics           `json:"metrics"`
	SLO           map[string]any    `json:"slo"`
	Links         map[string]string `json:"links,omitempty"`
	Summary       string            `json:"summary"`
}

// EmitOptions configures workshop-emit inputs.
type EmitOptions struct {
	OutDir           string
	Run              RunIdentity
	CandidatesPath   string
	CoveragePath     string
	AssessmentPath   string
	ProposalsPath    string
	RegistryPath     string
	HarnessPath      string
	PromptfooPath    string
	CorpusLabelsPath string
	EnforceAssessLLM bool
}

// Emit builds a workshop-snapshot directory from optional artifact paths.
func Emit(opts EmitOptions) (Latest, error) {
	if strings.TrimSpace(opts.OutDir) == "" {
		return Latest{}, fmt.Errorf("out dir is required")
	}
	if strings.TrimSpace(opts.Run.RunID) == "" {
		opts.Run.RunID = "local-" + time.Now().UTC().Format("20060102T150405Z")
	}
	if strings.TrimSpace(opts.Run.Timestamp) == "" {
		opts.Run.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(opts.Run.Workflow) == "" {
		opts.Run.Workflow = "local"
	}

	metrics := Metrics{SchemaVersion: MetricsSchemaVersion}
	links := map[string]string{}
	copied := map[string]string{}

	if opts.CandidatesPath != "" {
		var payload struct {
			CandidateCount    int `json:"candidate_count"`
			ConfidenceSummary struct {
				High   int `json:"high"`
				Medium int `json:"medium"`
				Low    int `json:"low"`
			} `json:"confidence_summary"`
			Candidates []struct {
				PatternHint string `json:"pattern_hint"`
			} `json:"candidates"`
		}
		if err := readJSON(opts.CandidatesPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("candidates: %w", err)
		}
		metrics.CandidateTotal = payload.CandidateCount
		metrics.ConfidenceHigh = payload.ConfidenceSummary.High
		metrics.ConfidenceMedium = payload.ConfidenceSummary.Medium
		metrics.ConfidenceLow = payload.ConfidenceSummary.Low
		if payload.CandidateCount > 0 {
			metrics.EscalationRate = float64(payload.ConfidenceSummary.Low) / float64(payload.CandidateCount)
			h := float64(payload.ConfidenceSummary.High) / float64(payload.CandidateCount)
			m := float64(payload.ConfidenceSummary.Medium) / float64(payload.CandidateCount)
			l := float64(payload.ConfidenceSummary.Low) / float64(payload.CandidateCount)
			metrics.ConfidenceShareHigh = &h
			metrics.ConfidenceShareMedium = &m
			metrics.ConfidenceShareLow = &l
		}
		manual := 0
		for _, c := range payload.Candidates {
			if c.PatternHint == "manual_review" {
				manual++
			}
		}
		if payload.CandidateCount > 0 {
			rate := float64(manual) / float64(payload.CandidateCount)
			metrics.EvidenceGapRate = &rate
		}
		if err := copyFile(opts.CandidatesPath, filepath.Join(opts.OutDir, "candidates.json")); err != nil {
			return Latest{}, err
		}
		copied["candidates"] = "candidates.json"
	}

	if opts.CoveragePath != "" {
		var payload struct {
			PendingReviewCount      int `json:"pending_review_count"`
			PendingHighConfidence   int `json:"pending_high_confidence"`
			PendingMediumConfidence int `json:"pending_medium_confidence"`
			PendingLowConfidence    int `json:"pending_low_confidence"`
			ApprovedCount           int `json:"approved_count"`
			UnboundCount            int `json:"unbound_count"`
			StaleCount              int `json:"stale_count"`
		}
		if err := readJSON(opts.CoveragePath, &payload); err != nil {
			return Latest{}, fmt.Errorf("coverage: %w", err)
		}
		metrics.ReviewBacklog = intPtr(payload.PendingReviewCount)
		metrics.PendingHigh = intPtr(payload.PendingHighConfidence)
		metrics.PendingMedium = intPtr(payload.PendingMediumConfidence)
		metrics.PendingLow = intPtr(payload.PendingLowConfidence)
		metrics.AssessUnbound = intPtr(payload.UnboundCount)
		metrics.AssessStale = intPtr(payload.StaleCount)
		covered := payload.ApprovedCount
		metrics.AssessCovered = &covered
		if err := copyFile(opts.CoveragePath, filepath.Join(opts.OutDir, "coverage.json")); err != nil {
			return Latest{}, err
		}
		copied["coverage"] = "coverage.json"
	}

	if opts.AssessmentPath != "" {
		var payload struct {
			Verdict         string `json:"verdict"`
			RuntimeLLMCalls int    `json:"runtime_llm_calls"`
		}
		if err := readJSON(opts.AssessmentPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("assessment: %w", err)
		}
		metrics.AssessVerdict = payload.Verdict
		metrics.RuntimeLLMCalls = intPtr(payload.RuntimeLLMCalls)
		if opts.EnforceAssessLLM || isAssessWorkflow(opts.Run.Workflow) {
			if payload.RuntimeLLMCalls != 0 {
				return Latest{}, fmt.Errorf("assess path invariant violated: runtime_llm_calls=%d", payload.RuntimeLLMCalls)
			}
		}
		if err := copyFile(opts.AssessmentPath, filepath.Join(opts.OutDir, "assessment.json")); err != nil {
			return Latest{}, err
		}
		copied["assessment"] = "assessment.json"
	}

	if opts.ProposalsPath != "" {
		var payload struct {
			ProposalCount    int     `json:"proposal_count"`
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			EstimatedCostUSD float64 `json:"estimated_cost_usd"`
			Provider         string  `json:"provider"`
			Model            string  `json:"model"`
		}
		if err := readJSON(opts.ProposalsPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("proposals: %w", err)
		}
		metrics.AssistProposalCount = intPtr(payload.ProposalCount)
		metrics.AssistPromptTokens = intPtr(payload.PromptTokens)
		metrics.AssistCompletionTokens = intPtr(payload.CompletionTokens)
		cost := payload.EstimatedCostUSD
		metrics.AssistCostUSD = &cost
		metrics.AssistProvider = payload.Provider
		metrics.AssistModel = payload.Model
		if err := copyFile(opts.ProposalsPath, filepath.Join(opts.OutDir, "proposals.json")); err != nil {
			return Latest{}, err
		}
		copied["proposals"] = "proposals.json"
	}

	if opts.RegistryPath != "" && opts.ProposalsPath != "" {
		proposals, err := obligations.LoadProposalIndex(opts.ProposalsPath)
		if err != nil {
			return Latest{}, fmt.Errorf("proposals for accept_rate: %w", err)
		}
		registry, err := obligations.LoadRegistry(opts.RegistryPath)
		if err != nil {
			return Latest{}, fmt.Errorf("registry for accept_rate: %w", err)
		}
		if rate, ok := obligations.AssistAcceptRate(proposals, registry); ok {
			metrics.AssistAcceptRate = &rate
		}
		if err := copyFile(opts.RegistryPath, filepath.Join(opts.OutDir, "registry.json")); err != nil {
			return Latest{}, err
		}
		copied["registry"] = "registry.json"
	} else if opts.RegistryPath != "" {
		if err := copyFile(opts.RegistryPath, filepath.Join(opts.OutDir, "registry.json")); err != nil {
			return Latest{}, err
		}
		copied["registry"] = "registry.json"
	}

	if opts.HarnessPath != "" {
		var payload struct {
			Precision          float64                       `json:"precision"`
			Recall             float64                       `json:"recall"`
			RecallByConfidence map[string]map[string]float64 `json:"recall_by_confidence"`
		}
		if err := readJSON(opts.HarnessPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("harness: %w", err)
		}
		p, r := payload.Precision, payload.Recall
		metrics.HarnessPrecision = &p
		metrics.HarnessRecall = &r
		for band, ptr := range map[string]**float64{
			"high":   &metrics.HarnessRecallHigh,
			"medium": &metrics.HarnessRecallMedium,
			"low":    &metrics.HarnessRecallLow,
		} {
			if row, ok := payload.RecallByConfidence[band]; ok {
				if v, ok := row["recall"]; ok {
					val := v
					*ptr = &val
				}
			}
		}
		if err := copyFile(opts.HarnessPath, filepath.Join(opts.OutDir, "harness.json")); err != nil {
			return Latest{}, err
		}
		copied["harness"] = "harness.json"
	}

	if opts.PromptfooPath != "" {
		var payload map[string]any
		if err := readJSON(opts.PromptfooPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("promptfoo: %w", err)
		}
		passRate, mode := promptfooSummary(payload)
		metrics.PromptfooPassRate = passRate
		metrics.PromptfooMode = mode
		if err := copyFile(opts.PromptfooPath, filepath.Join(opts.OutDir, "promptfoo.json")); err != nil {
			return Latest{}, err
		}
		copied["promptfoo"] = "promptfoo.json"
	}

	if opts.CorpusLabelsPath != "" {
		var payload struct {
			CorpusSize int   `json:"corpus_size"`
			Expected   []any `json:"expected_candidates"`
		}
		if err := readJSON(opts.CorpusLabelsPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("corpus labels: %w", err)
		}
		size := payload.CorpusSize
		if size == 0 {
			size = len(payload.Expected)
		}
		metrics.CorpusSize = &size
	}

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return Latest{}, err
	}

	slo := map[string]any{
		"assess_runtime_llm_calls_zero": metrics.RuntimeLLMCalls == nil || *metrics.RuntimeLLMCalls == 0,
		"promptfoo_offline_pass":        metrics.PromptfooPassRate == nil || (metrics.PromptfooMode != "live" && *metrics.PromptfooPassRate >= 1.0),
	}
	if metrics.CandidateTotal > 0 {
		slo["high_confidence_share"] = float64(metrics.ConfidenceHigh) / float64(metrics.CandidateTotal)
	}

	latest := Latest{
		SchemaVersion: SnapshotSchemaVersion,
		Run:           opts.Run,
		Metrics:       metrics,
		SLO:           slo,
		Links:         links,
		Summary:       summarize(opts.Run, metrics),
	}
	for k, v := range copied {
		latest.Links[k] = v
	}

	if err := writeJSON(filepath.Join(opts.OutDir, "run.json"), opts.Run); err != nil {
		return Latest{}, err
	}
	if err := writeJSON(filepath.Join(opts.OutDir, "metrics.json"), metrics); err != nil {
		return Latest{}, err
	}
	if err := writeJSON(filepath.Join(opts.OutDir, "latest.json"), latest); err != nil {
		return Latest{}, err
	}
	return latest, nil
}

// MetricsRow is one append-only runs.jsonl record.
type MetricsRow struct {
	Run     RunIdentity `json:"run"`
	Metrics Metrics     `json:"metrics"`
}

func summarize(run RunIdentity, m Metrics) string {
	parts := []string{fmt.Sprintf("workflow=%s", run.Workflow)}
	if m.AssessVerdict != "" {
		parts = append(parts, "verdict="+m.AssessVerdict)
	}
	parts = append(parts, fmt.Sprintf("candidates=%d (high=%d medium=%d low=%d)",
		m.CandidateTotal, m.ConfidenceHigh, m.ConfidenceMedium, m.ConfidenceLow))
	if m.AssistProposalCount != nil {
		parts = append(parts, fmt.Sprintf("proposals=%d", *m.AssistProposalCount))
	}
	return strings.Join(parts, "; ")
}

func isAssessWorkflow(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "assess") || n == "ci" || strings.Contains(n, "adr assessment")
}

func promptfooSummary(payload map[string]any) (*float64, string) {
	mode := "offline"
	if v, ok := payload["mode"].(string); ok && v != "" {
		mode = v
	}
	if results, ok := payload["results"].(map[string]any); ok {
		if stats, ok := results["stats"].(map[string]any); ok {
			successes := asFloat(stats["successes"])
			failures := asFloat(stats["failures"])
			total := successes + failures
			if total > 0 {
				rate := successes / total
				return &rate, mode
			}
		}
	}
	if v, ok := payload["pass_rate"]; ok {
		rate := asFloat(v)
		return &rate, mode
	}
	return nil, mode
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func intPtr(v int) *int { return &v }

func readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
