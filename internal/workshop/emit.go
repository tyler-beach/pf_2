package workshop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/obligations"
	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
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

	// Wilson score intervals (95%) for key rates — workshop ± display.
	EscalationRateLo          *float64 `json:"kpi.escalation.rate.ci_low,omitempty"`
	EscalationRateHi          *float64 `json:"kpi.escalation.rate.ci_high,omitempty"`
	ConfidenceShareHighLo     *float64 `json:"kpi.confidence.share.high.ci_low,omitempty"`
	ConfidenceShareHighHi     *float64 `json:"kpi.confidence.share.high.ci_high,omitempty"`
	ConfidenceShareMediumLo   *float64 `json:"kpi.confidence.share.medium.ci_low,omitempty"`
	ConfidenceShareMediumHi   *float64 `json:"kpi.confidence.share.medium.ci_high,omitempty"`
	ConfidenceShareLowLo      *float64 `json:"kpi.confidence.share.low.ci_low,omitempty"`
	ConfidenceShareLowHi      *float64 `json:"kpi.confidence.share.low.ci_high,omitempty"`
	EscalationTechSpecLo      *float64 `json:"kpi.escalation.rate.tech_spec.ci_low,omitempty"`
	EscalationTechSpecHi      *float64 `json:"kpi.escalation.rate.tech_spec.ci_high,omitempty"`
	EscalationCodeSnippetLo   *float64 `json:"kpi.escalation.rate.code_snippet.ci_low,omitempty"`
	EscalationCodeSnippetHi   *float64 `json:"kpi.escalation.rate.code_snippet.ci_high,omitempty"`
	HarnessPrecisionLo        *float64 `json:"kpi.harness.precision.ci_low,omitempty"`
	HarnessPrecisionHi        *float64 `json:"kpi.harness.precision.ci_high,omitempty"`
	ConfidenceMethod          string   `json:"kpi.confidence.method,omitempty"`
	ConfidenceCoverageTarget  *float64 `json:"kpi.confidence.coverage_target,omitempty"`
	ConfidenceMeanP           *float64 `json:"kpi.confidence.mean_p,omitempty"`
	ConfidenceMeanPTechSpec   *float64 `json:"kpi.confidence.mean_p.tech_spec,omitempty"`
	ConfidenceMeanPCodeSnippet *float64 `json:"kpi.confidence.mean_p.code_snippet,omitempty"`
	ConfidenceCalibratedCount *int     `json:"kpi.confidence.calibrated_count,omitempty"`
	ConfidenceHeuristicCount  *int     `json:"kpi.confidence.heuristic_count,omitempty"`
	ConfidenceParamsVersion   string   `json:"kpi.confidence.params_version,omitempty"`
	ConfidenceGoldSize        *int     `json:"kpi.confidence.gold_size,omitempty"`
	ConfidenceEmpiricalCoverage *float64 `json:"kpi.confidence.empirical_coverage,omitempty"`
	ConfidenceCoverageGap     *float64 `json:"kpi.confidence.coverage_gap,omitempty"`
	CILevel                   float64  `json:"kpi.ci.level,omitempty"`
	CIMethod                  string   `json:"kpi.ci.method,omitempty"`

	HarnessPrecision          *float64 `json:"kpi.harness.precision,omitempty"`
	HarnessRecall             *float64 `json:"kpi.harness.recall,omitempty"`
	HarnessRecallHigh         *float64 `json:"kpi.harness.recall.high,omitempty"`
	HarnessRecallMedium       *float64 `json:"kpi.harness.recall.medium,omitempty"`
	HarnessRecallLow          *float64 `json:"kpi.harness.recall.low,omitempty"`
	HarnessPrecisionHigh      *float64 `json:"kpi.harness.precision.high,omitempty"`
	HarnessPrecisionMedium    *float64 `json:"kpi.harness.precision.medium,omitempty"`
	HarnessPrecisionLow       *float64 `json:"kpi.harness.precision.low,omitempty"`
	HarnessPrecisionTechSpec  *float64 `json:"kpi.harness.precision.tech_spec,omitempty"`
	HarnessPrecisionCodeSnippet *float64 `json:"kpi.harness.precision.code_snippet,omitempty"`
	HarnessRecallTechSpec     *float64 `json:"kpi.harness.recall.tech_spec,omitempty"`
	HarnessRecallCodeSnippet  *float64 `json:"kpi.harness.recall.code_snippet,omitempty"`
	HarnessFalsePositives     *int     `json:"kpi.harness.false_positives,omitempty"`
	HarnessFPRate             *float64 `json:"kpi.harness.fp_rate,omitempty"`

	ReviewBacklog    *int     `json:"kpi.review.backlog,omitempty"`
	PendingHigh      *int     `json:"kpi.review.pending_high,omitempty"`
	PendingMedium    *int     `json:"kpi.review.pending_medium,omitempty"`
	PendingLow       *int     `json:"kpi.review.pending_low,omitempty"`
	AssistAcceptRate *float64 `json:"kpi.review.accept_rate,omitempty"`
	AssistRejectRate *float64 `json:"kpi.review.reject_rate,omitempty"`

	AssessVerdict            string   `json:"kpi.assess.verdict,omitempty"`
	AssessSubmissionKind     string   `json:"kpi.assess.submission_kind,omitempty"`
	AssessCovered            *int     `json:"kpi.assess.covered,omitempty"`
	AssessUnbound            *int     `json:"kpi.assess.unbound,omitempty"`
	AssessStale              *int     `json:"kpi.assess.stale,omitempty"`
	AssessRequirementCount   *int     `json:"kpi.assess.requirement_count,omitempty"`
	AssessApplicableCount    *int     `json:"kpi.assess.applicable_count,omitempty"`
	AssessNARate             *float64 `json:"kpi.assess.na_rate,omitempty"`
	AssessPassShare          *float64 `json:"kpi.assess.pass_share,omitempty"`
	AssessFailShare          *float64 `json:"kpi.assess.fail_share,omitempty"`
	AssessIndeterminateShare *float64 `json:"kpi.assess.indeterminate_share,omitempty"`
	AssessWarnShare          *float64 `json:"kpi.assess.warn_share,omitempty"`
	AssessFunnelInventory    *int     `json:"kpi.assess.funnel.inventory,omitempty"`
	AssessFunnelAccepted     *int     `json:"kpi.assess.funnel.accepted,omitempty"`
	AssessFunnelInterpreted  *int     `json:"kpi.assess.funnel.interpreted,omitempty"`
	AssessFunnelBound        *int     `json:"kpi.assess.funnel.bound,omitempty"`
	AssessFunnelEvidenceComplete *int `json:"kpi.assess.funnel.evidence_complete,omitempty"`
	AssessFunnelAcceptedRate *float64 `json:"kpi.assess.funnel.accepted_rate,omitempty"`
	AssessFunnelInterpretedRate *float64 `json:"kpi.assess.funnel.interpreted_rate,omitempty"`
	AssessFunnelBoundRate    *float64 `json:"kpi.assess.funnel.bound_rate,omitempty"`
	AssessFunnelEvidenceCompleteRate *float64 `json:"kpi.assess.funnel.evidence_complete_rate,omitempty"`
	RuntimeLLMCalls          *int     `json:"kpi.invariant.llm_calls,omitempty"`
	SubmissionTechSpec       *int     `json:"kpi.submission.tech_spec_count,omitempty"`
	SubmissionCodeSnippet    *int     `json:"kpi.submission.code_snippet_count,omitempty"`
	SubmissionUnspecified    *int     `json:"kpi.submission.unspecified_count,omitempty"`
	SubmissionUnspecifiedRate *float64 `json:"kpi.submission.unspecified_rate,omitempty"`
	EscalationTechSpec       *float64 `json:"kpi.escalation.rate.tech_spec,omitempty"`
	EscalationCodeSnippet    *float64 `json:"kpi.escalation.rate.code_snippet,omitempty"`

	AssistProposalCount    *int     `json:"kpi.assist.proposal_count,omitempty"`
	AssistPromptTokens     *int     `json:"kpi.assist.prompt_tokens,omitempty"`
	AssistCompletionTokens *int     `json:"kpi.assist.completion_tokens,omitempty"`
	AssistCostUSD          *float64 `json:"kpi.assist.cost_usd,omitempty"`
	AssistProvider         string   `json:"kpi.assist.provider,omitempty"`
	AssistModel            string   `json:"kpi.assist.model,omitempty"`
	CostPerAcceptedProposal *float64 `json:"kpi.cost.per_accepted_proposal,omitempty"`
	CostPerFail            *float64 `json:"kpi.cost.per_fail,omitempty"`
	PromptfooPassRate      *float64 `json:"kpi.promptfoo.pass_rate,omitempty"`
	PromptfooMode          string   `json:"promptfoo_mode,omitempty"`

	EvidenceGapRate           *float64 `json:"kpi.evidence.gap_rate,omitempty"`
	EvidenceGapRateTechSpec   *float64 `json:"kpi.evidence.gap_rate.tech_spec,omitempty"`
	EvidenceGapRateCodeSnippet *float64 `json:"kpi.evidence.gap_rate.code_snippet,omitempty"`
	CorpusSize                *int     `json:"kpi.corpus.size,omitempty"`

	// ADR flagging rollup (from assessment requirements ± batch).
	ADRTouched            *int     `json:"kpi.adr.touched,omitempty"`
	ADRAssessed           *int     `json:"kpi.adr.assessed,omitempty"`
	ADRFlagRate           *float64 `json:"kpi.adr.flag_rate,omitempty"`
	ADRFlagRateLo         *float64 `json:"kpi.adr.flag_rate.ci_low,omitempty"`
	ADRFlagRateHi         *float64 `json:"kpi.adr.flag_rate.ci_high,omitempty"`
	ADRTopFlagRate        *float64 `json:"kpi.adr.top_flag_rate,omitempty"`
	ADRTopFlagRateADR     string   `json:"kpi.adr.top_flag_rate_adr,omitempty"`
	ADRFailCount          *int     `json:"kpi.adr.fail_count,omitempty"`
	ADRPassCount          *int     `json:"kpi.adr.pass_count,omitempty"`
	ADRIndeterminateCount *int     `json:"kpi.adr.indeterminate_count,omitempty"`
	ADRMostFail           string   `json:"kpi.adr.most_fail,omitempty"`
	ADRMostAligned        string   `json:"kpi.adr.most_aligned,omitempty"`

	// Rule / analyzer hit rates (OpenGrep/Checkov findings).
	RuleDistinctCount *int    `json:"kpi.rule.distinct_count,omitempty"`
	RuleHitTotal      *int    `json:"kpi.rule.hit_total,omitempty"`
	RuleFlagTotal     *int    `json:"kpi.rule.flag_total,omitempty"`
	RuleTopRuleID     string  `json:"kpi.rule.top_rule_id,omitempty"`
	RuleTopHitCount   *int    `json:"kpi.rule.top_hit_count,omitempty"`
	RuleTopFlagRuleID string  `json:"kpi.rule.top_flag_rule_id,omitempty"`
	RuleTopFlagCount  *int    `json:"kpi.rule.top_flag_count,omitempty"`

	PublishAgeMinutes *int `json:"kpi.publish.age_minutes,omitempty"`
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
	// SubmissionsIndexPath is an optional workshop submissions results index
	// (submissions.json) whose per-submission assessment_path files are rolled
	// into adr_flagging.json. Prefer this for batch snapshots when the rollup
	// assessment.json has empty requirements.
	SubmissionsIndexPath string
	ProposalsPath        string
	RegistryPath         string
	HarnessPath          string
	PromptfooPath        string
	CorpusLabelsPath     string
	EnforceAssessLLM     bool
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
			Calibration struct {
				Method           string  `json:"method"`
				Coverage         float64 `json:"coverage"`
				GoldExampleCount int     `json:"gold_example_count"`
				ParamsVersion    string  `json:"params_version"`
			} `json:"calibration"`
			Candidates []struct {
				PatternHint        string   `json:"pattern_hint"`
				SubmissionKind     string   `json:"submission_kind"`
				Confidence         string   `json:"confidence"`
				ConfidenceP        float64  `json:"confidence_p"`
				ConfidenceMethod   string   `json:"confidence_method"`
				ConfidenceCoverage float64  `json:"confidence_coverage"`
				PredictionSet      []string `json:"prediction_set"`
			} `json:"candidates"`
		}
		if err := readJSON(opts.CandidatesPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("candidates: %w", err)
		}
		metrics.CandidateTotal = payload.CandidateCount
		metrics.ConfidenceHigh = payload.ConfidenceSummary.High
		metrics.ConfidenceMedium = payload.ConfidenceSummary.Medium
		metrics.ConfidenceLow = payload.ConfidenceSummary.Low
		metrics.CILevel = 0.95
		metrics.CIMethod = "wilson"
		if payload.CandidateCount > 0 {
			metrics.EscalationRate = float64(payload.ConfidenceSummary.Low) / float64(payload.CandidateCount)
			h := float64(payload.ConfidenceSummary.High) / float64(payload.CandidateCount)
			m := float64(payload.ConfidenceSummary.Medium) / float64(payload.CandidateCount)
			l := float64(payload.ConfidenceSummary.Low) / float64(payload.CandidateCount)
			metrics.ConfidenceShareHigh = &h
			metrics.ConfidenceShareMedium = &m
			metrics.ConfidenceShareLow = &l
			escCI := calibration.WilsonInterval(payload.ConfidenceSummary.Low, payload.CandidateCount, 0.95)
			metrics.EscalationRateLo = floatPtr(escCI.Low)
			metrics.EscalationRateHi = floatPtr(escCI.High)
			hiCI := calibration.WilsonInterval(payload.ConfidenceSummary.High, payload.CandidateCount, 0.95)
			metrics.ConfidenceShareHighLo = floatPtr(hiCI.Low)
			metrics.ConfidenceShareHighHi = floatPtr(hiCI.High)
			medCI := calibration.WilsonInterval(payload.ConfidenceSummary.Medium, payload.CandidateCount, 0.95)
			metrics.ConfidenceShareMediumLo = floatPtr(medCI.Low)
			metrics.ConfidenceShareMediumHi = floatPtr(medCI.High)
			loCI := calibration.WilsonInterval(payload.ConfidenceSummary.Low, payload.CandidateCount, 0.95)
			metrics.ConfidenceShareLowLo = floatPtr(loCI.Low)
			metrics.ConfidenceShareLowHi = floatPtr(loCI.High)
		}
		manual := 0
		techSpec, codeSnippet := 0, 0
		techSpecLow, codeSnippetLow := 0, 0
		calibrated, heuristic := 0, 0
		sumP := 0.0
		method := payload.Calibration.Method
		coverage := payload.Calibration.Coverage
		kindCands := make([]kindCand, 0, len(payload.Candidates))
		for _, c := range payload.Candidates {
			if c.PatternHint == "manual_review" {
				manual++
			}
			switch c.SubmissionKind {
			case "tech_spec":
				techSpec++
				if c.Confidence == "low" {
					techSpecLow++
				}
			case "code_snippet":
				codeSnippet++
				if c.Confidence == "low" {
					codeSnippetLow++
				}
			}
			sumP += c.ConfidenceP
			cm := c.ConfidenceMethod
			// Empty method is heuristic; do not inherit batch-level method for counts.
			if cm == calibration.MethodPlattConformal {
				calibrated++
			} else {
				heuristic++
			}
			if method == "" && cm != "" {
				method = cm
			}
			if coverage <= 0 && c.ConfidenceCoverage > 0 {
				coverage = c.ConfidenceCoverage
			}
			kindCands = append(kindCands, kindCand{
				SubmissionKind: c.SubmissionKind,
				PatternHint:    c.PatternHint,
				ConfidenceP:    c.ConfidenceP,
				PredictionSet:  c.PredictionSet,
				Confidence:     c.Confidence,
			})
		}
		if method != "" {
			metrics.ConfidenceMethod = method
		}
		if coverage > 0 {
			metrics.ConfidenceCoverageTarget = floatPtr(coverage)
		}
		if payload.Calibration.ParamsVersion != "" {
			metrics.ConfidenceParamsVersion = payload.Calibration.ParamsVersion
		}
		if payload.Calibration.GoldExampleCount > 0 {
			metrics.ConfidenceGoldSize = intPtr(payload.Calibration.GoldExampleCount)
		}
		if len(payload.Candidates) > 0 {
			meanP := sumP / float64(len(payload.Candidates))
			metrics.ConfidenceMeanP = &meanP
		}
		metrics.ConfidenceCalibratedCount = &calibrated
		metrics.ConfidenceHeuristicCount = &heuristic
		if emp := EmpiricalPredictionSetCoverage(kindCands); emp != nil {
			metrics.ConfidenceEmpiricalCoverage = emp
			if coverage > 0 {
				metrics.ConfidenceCoverageGap = floatPtr(*emp - coverage)
			}
		}
		kq := ComputeKindQualityStats(kindCands, payload.CandidateCount)
		metrics.ConfidenceMeanPTechSpec = kq.TechSpecMeanP
		metrics.ConfidenceMeanPCodeSnippet = kq.CodeSnippetMeanP
		metrics.EvidenceGapRateTechSpec = kq.TechSpecGapRate
		metrics.EvidenceGapRateCodeSnippet = kq.CodeSnippetGapRate
		if kq.UnspecifiedCount > 0 || kq.UnspecifiedRate != nil {
			metrics.SubmissionUnspecified = intPtr(kq.UnspecifiedCount)
			metrics.SubmissionUnspecifiedRate = kq.UnspecifiedRate
		}
		if techSpec > 0 {
			metrics.SubmissionTechSpec = &techSpec
			rate := float64(techSpecLow) / float64(techSpec)
			metrics.EscalationTechSpec = &rate
			ci := calibration.WilsonInterval(techSpecLow, techSpec, 0.95)
			metrics.EscalationTechSpecLo = floatPtr(ci.Low)
			metrics.EscalationTechSpecHi = floatPtr(ci.High)
		}
		if codeSnippet > 0 {
			metrics.SubmissionCodeSnippet = &codeSnippet
			rate := float64(codeSnippetLow) / float64(codeSnippet)
			metrics.EscalationCodeSnippet = &rate
			ci := calibration.WilsonInterval(codeSnippetLow, codeSnippet, 0.95)
			metrics.EscalationCodeSnippetLo = floatPtr(ci.Low)
			metrics.EscalationCodeSnippetHi = floatPtr(ci.High)
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

	var flaggingInputsForRules []ADRFlaggingInput

	if opts.AssessmentPath != "" {
		var payload struct {
			Verdict                    string         `json:"verdict"`
			RuntimeLLMCalls            int            `json:"runtime_llm_calls"`
			SubmissionKind             string         `json:"submission_kind"`
			RequirementCount           int            `json:"requirement_count"`
			ApplicableRequirementCount int            `json:"applicable_requirement_count"`
			Counts                     map[string]int `json:"counts"`
			CorpusCoverage             struct {
				InventoryADRs           int            `json:"inventory_adrs"`
				AcceptedADRs            int            `json:"accepted_adrs"`
				InterpretedRequirements int            `json:"interpreted_requirements"`
				BoundRequirements       int            `json:"bound_requirements"`
				EvidenceComplete        int            `json:"evidence_complete"`
				OutcomeCounts           map[string]int `json:"outcome_counts"`
			} `json:"corpus_coverage"`
			Requirements []struct {
				ADRID         string `json:"adr_id"`
				Title         string `json:"title"`
				RequirementID string `json:"requirement_id"`
				Status        string `json:"status"`
				Reason        string `json:"reason"`
				Findings      []struct {
					RuleID   string `json:"rule_id"`
					Analyzer string `json:"analyzer"`
					Path     string `json:"path"`
					Message  string `json:"message"`
				} `json:"findings"`
			} `json:"requirements"`
		}
		if err := readJSON(opts.AssessmentPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("assessment: %w", err)
		}
		metrics.AssessVerdict = payload.Verdict
		metrics.AssessSubmissionKind = payload.SubmissionKind
		metrics.RuntimeLLMCalls = intPtr(payload.RuntimeLLMCalls)
		if opts.EnforceAssessLLM || isAssessWorkflow(opts.Run.Workflow) {
			if payload.RuntimeLLMCalls != 0 {
				return Latest{}, fmt.Errorf("assess path invariant violated: runtime_llm_calls=%d", payload.RuntimeLLMCalls)
			}
		}
		counts := payload.Counts
		if len(counts) == 0 {
			counts = payload.CorpusCoverage.OutcomeCounts
		}
		if len(counts) == 0 && len(payload.Requirements) > 0 {
			counts = map[string]int{}
			for _, r := range payload.Requirements {
				counts[r.Status]++
			}
		}
		mix := ComputeAssessOutcomeMix(counts, payload.RequirementCount, payload.ApplicableRequirementCount)
		if mix.RequirementCount > 0 {
			metrics.AssessRequirementCount = intPtr(mix.RequirementCount)
		}
		metrics.AssessApplicableCount = intPtr(mix.ApplicableCount)
		metrics.AssessNARate = mix.NARate
		metrics.AssessPassShare = mix.PassShare
		metrics.AssessFailShare = mix.FailShare
		metrics.AssessIndeterminateShare = mix.IndeterminateShare
		metrics.AssessWarnShare = mix.WarnShare

		funnel := ComputeCorpusFunnelRates(
			payload.CorpusCoverage.InventoryADRs,
			payload.CorpusCoverage.AcceptedADRs,
			payload.CorpusCoverage.InterpretedRequirements,
			payload.CorpusCoverage.BoundRequirements,
			payload.CorpusCoverage.EvidenceComplete,
		)
		if funnel.Inventory > 0 {
			metrics.AssessFunnelInventory = intPtr(funnel.Inventory)
			metrics.AssessFunnelAccepted = intPtr(funnel.Accepted)
			metrics.AssessFunnelInterpreted = intPtr(funnel.Interpreted)
			metrics.AssessFunnelBound = intPtr(funnel.Bound)
			metrics.AssessFunnelEvidenceComplete = intPtr(funnel.EvidenceComplete)
			metrics.AssessFunnelAcceptedRate = funnel.AcceptedRate
			metrics.AssessFunnelInterpretedRate = funnel.InterpretedRate
			metrics.AssessFunnelBoundRate = funnel.BoundRate
			metrics.AssessFunnelEvidenceCompleteRate = funnel.EvidenceCompleteRate
		}

		if err := copyFile(opts.AssessmentPath, filepath.Join(opts.OutDir, "assessment.json")); err != nil {
			return Latest{}, err
		}
		copied["assessment"] = "assessment.json"

		if len(payload.Requirements) > 0 && opts.SubmissionsIndexPath == "" {
			reqs := make([]ADRFlaggingInputRequirement, 0, len(payload.Requirements))
			for _, r := range payload.Requirements {
				findings := make([]ADRFlaggingRule, 0, len(r.Findings))
				for _, f := range r.Findings {
					findings = append(findings, ADRFlaggingRule{
						RuleID: f.RuleID, Analyzer: f.Analyzer, Path: f.Path, Message: f.Message,
					})
				}
				reqs = append(reqs, ADRFlaggingInputRequirement{
					ADRID: r.ADRID, Title: r.Title, RequirementID: r.RequirementID,
					Status: r.Status, Reason: r.Reason, Findings: findings,
				})
			}
			inputs := []ADRFlaggingInput{{
				RunID: opts.Run.RunID, SubmissionKind: payload.SubmissionKind, Requirements: reqs,
			}}
			flaggingInputsForRules = inputs
			report := BuildADRFlagging(opts.Run.RunID, inputs)
			if err := writeADRFlaggingArtifact(opts.OutDir, &metrics, links, copied, report); err != nil {
				return Latest{}, err
			}
		}
	}

	if opts.SubmissionsIndexPath != "" {
		inputs, err := LoadADRFlaggingInputsFromSubmissions(opts.SubmissionsIndexPath, opts.Run.RunID)
		if err != nil {
			return Latest{}, fmt.Errorf("adr flagging submissions: %w", err)
		}
		if len(inputs) > 0 {
			flaggingInputsForRules = inputs
			report := BuildADRFlagging(opts.Run.RunID, inputs)
			if err := writeADRFlaggingArtifact(opts.OutDir, &metrics, links, copied, report); err != nil {
				return Latest{}, err
			}
			// Batch rollups often set submission_kind=mixed on the assessment envelope.
			if metrics.AssessSubmissionKind == "" {
				kinds := map[string]struct{}{}
				for _, in := range inputs {
					if k := strings.TrimSpace(in.SubmissionKind); k != "" {
						kinds[k] = struct{}{}
					}
				}
				switch len(kinds) {
				case 0:
				case 1:
					for k := range kinds {
						metrics.AssessSubmissionKind = k
					}
				default:
					metrics.AssessSubmissionKind = "mixed"
				}
			}
		}
	}

	if len(flaggingInputsForRules) > 0 {
		rules := BuildRuleHits(opts.Run.RunID, flaggingInputsForRules)
		if err := writeRuleHitsArtifact(opts.OutDir, &metrics, links, copied, rules); err != nil {
			return Latest{}, err
		}
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

	acceptedAssist := 0
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
		if rate, ok := obligations.AssistRejectRate(proposals, registry); ok {
			metrics.AssistRejectRate = &rate
		}
		if approved, _, ok := obligations.AssistDecisionTotals(proposals, registry); ok {
			acceptedAssist = approved
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
			Precision             float64                       `json:"precision"`
			Recall                float64                       `json:"recall"`
			MatchedCandidates     int                           `json:"matched_candidates"`
			FalsePositives        int                           `json:"false_positives"`
			RecallByConfidence    map[string]map[string]float64 `json:"recall_by_confidence"`
			PrecisionByConfidence map[string]map[string]float64 `json:"precision_by_confidence"`
			BySubmissionKind      map[string]struct {
				Precision *float64 `json:"precision"`
				Recall    *float64 `json:"recall"`
			} `json:"by_submission_kind"`
		}
		if err := readJSON(opts.HarnessPath, &payload); err != nil {
			return Latest{}, fmt.Errorf("harness: %w", err)
		}
		p, r := payload.Precision, payload.Recall
		metrics.HarnessPrecision = &p
		metrics.HarnessRecall = &r
		metrics.HarnessFalsePositives = intPtr(payload.FalsePositives)
		denom := payload.MatchedCandidates + payload.FalsePositives
		if denom > 0 {
			ci := calibration.WilsonInterval(payload.MatchedCandidates, denom, 0.95)
			metrics.HarnessPrecisionLo = floatPtr(ci.Low)
			metrics.HarnessPrecisionHi = floatPtr(ci.High)
			metrics.HarnessFPRate = floatPtr(float64(payload.FalsePositives) / float64(denom))
		}
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
		for band, ptr := range map[string]**float64{
			"high":   &metrics.HarnessPrecisionHigh,
			"medium": &metrics.HarnessPrecisionMedium,
			"low":    &metrics.HarnessPrecisionLow,
		} {
			if row, ok := payload.PrecisionByConfidence[band]; ok {
				if v, ok := row["precision"]; ok {
					val := v
					*ptr = &val
				}
			}
		}
		if row, ok := payload.BySubmissionKind["tech_spec"]; ok {
			metrics.HarnessPrecisionTechSpec = row.Precision
			metrics.HarnessRecallTechSpec = row.Recall
		}
		if row, ok := payload.BySubmissionKind["code_snippet"]; ok {
			metrics.HarnessPrecisionCodeSnippet = row.Precision
			metrics.HarnessRecallCodeSnippet = row.Recall
		}
		if err := copyFile(opts.HarnessPath, filepath.Join(opts.OutDir, "harness.json")); err != nil {
			return Latest{}, err
		}
		copied["harness"] = "harness.json"
	}

	if metrics.AssistCostUSD != nil {
		failN := 0
		if metrics.ADRFailCount != nil {
			failN = *metrics.ADRFailCount
		}
		perAcc, perFail := CostPerUsefulOutcome(*metrics.AssistCostUSD, acceptedAssist, failN)
		metrics.CostPerAcceptedProposal = perAcc
		metrics.CostPerFail = perFail
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

	ageMin, freshOK := PublishFreshnessSLO(opts.Run.Timestamp, time.Now().UTC())
	metrics.PublishAgeMinutes = intPtr(ageMin)
	slo := map[string]any{
		"assess_runtime_llm_calls_zero":      metrics.RuntimeLLMCalls == nil || *metrics.RuntimeLLMCalls == 0,
		"promptfoo_offline_pass":             metrics.PromptfooPassRate == nil || (metrics.PromptfooMode != "live" && *metrics.PromptfooPassRate >= 1.0),
		"publish_freshness_ok":               freshOK,
		"publish_age_minutes":                ageMin,
		"publish_freshness_max_age_minutes": PublishFreshnessMaxAgeMinutes,
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

func floatPtr(v float64) *float64 { return &v }

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
