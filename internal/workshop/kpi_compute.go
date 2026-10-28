package workshop

import (
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

// PublishFreshnessMaxAgeMinutes is the SLO threshold for publish/run freshness
// (aligned with workshop UI stale threshold of ~3h).
const PublishFreshnessMaxAgeMinutes = 180

// AssessOutcomeMix holds requirement status mix KPIs.
type AssessOutcomeMix struct {
	RequirementCount  int
	ApplicableCount   int
	NACount           int
	PassCount         int
	FailCount         int
	IndeterminateCount int
	WarnCount         int
	NARate            *float64
	PassShare         *float64
	FailShare         *float64
	IndeterminateShare *float64
	WarnShare         *float64
}

// ComputeAssessOutcomeMix aggregates assessment counts / requirements into mix KPIs.
// Shares are among applicable (non-N/A) requirements when applicable > 0;
// na_rate is N/A / total requirements.
func ComputeAssessOutcomeMix(counts map[string]int, requirementTotal, applicableHint int) AssessOutcomeMix {
	out := AssessOutcomeMix{}
	if counts == nil {
		counts = map[string]int{}
	}
	total := 0
	for status, n := range counts {
		total += n
		switch strings.ToUpper(strings.TrimSpace(status)) {
		case "NOT_APPLICABLE":
			out.NACount += n
		case "PASS":
			out.PassCount += n
		case "FAIL":
			out.FailCount += n
		case "INDETERMINATE":
			out.IndeterminateCount += n
		case "WARN":
			out.WarnCount += n
		}
	}
	if requirementTotal > 0 {
		out.RequirementCount = requirementTotal
	} else {
		out.RequirementCount = total
	}
	if applicableHint > 0 {
		out.ApplicableCount = applicableHint
	} else {
		out.ApplicableCount = out.RequirementCount - out.NACount
		if out.ApplicableCount < 0 {
			out.ApplicableCount = 0
		}
	}
	if out.RequirementCount > 0 {
		out.NARate = floatPtr(float64(out.NACount) / float64(out.RequirementCount))
	}
	if out.ApplicableCount > 0 {
		out.PassShare = floatPtr(float64(out.PassCount) / float64(out.ApplicableCount))
		out.FailShare = floatPtr(float64(out.FailCount) / float64(out.ApplicableCount))
		out.IndeterminateShare = floatPtr(float64(out.IndeterminateCount) / float64(out.ApplicableCount))
		out.WarnShare = floatPtr(float64(out.WarnCount) / float64(out.ApplicableCount))
	}
	return out
}

// CorpusFunnelRates converts absolute corpus_coverage counts into stage rates
// relative to inventory (or the first non-zero upstream stage).
type CorpusFunnelRates struct {
	Inventory         int
	Accepted          int
	Interpreted       int
	Bound             int
	EvidenceComplete  int
	AcceptedRate      *float64
	InterpretedRate   *float64
	BoundRate         *float64
	EvidenceCompleteRate *float64
}

// ComputeCorpusFunnelRates builds inventory→…→evidence_complete rates.
func ComputeCorpusFunnelRates(inventory, accepted, interpreted, bound, evidenceComplete int) CorpusFunnelRates {
	out := CorpusFunnelRates{
		Inventory:        inventory,
		Accepted:         accepted,
		Interpreted:      interpreted,
		Bound:            bound,
		EvidenceComplete: evidenceComplete,
	}
	base := inventory
	if base <= 0 {
		return out
	}
	out.AcceptedRate = floatPtr(float64(accepted) / float64(base))
	out.InterpretedRate = floatPtr(float64(interpreted) / float64(base))
	out.BoundRate = floatPtr(float64(bound) / float64(base))
	out.EvidenceCompleteRate = floatPtr(float64(evidenceComplete) / float64(base))
	return out
}

// KindQualityStats holds extract-quality splits by submission_kind.
type KindQualityStats struct {
	TechSpecMeanP       *float64
	CodeSnippetMeanP    *float64
	TechSpecGapRate     *float64
	CodeSnippetGapRate  *float64
	UnspecifiedCount    int
	UnspecifiedRate     *float64
	TechSpecCount       int
	CodeSnippetCount    int
}

type kindCand struct {
	SubmissionKind string
	PatternHint    string
	ConfidenceP    float64
	PredictionSet  []string
	Confidence     string
}

// ComputeKindQualityStats derives mean_p / evidence gap / unspecified rate by kind.
func ComputeKindQualityStats(candidates []kindCand, candidateTotal int) KindQualityStats {
	var out KindQualityStats
	var techSum, codeSum float64
	var techN, codeN, techManual, codeManual, unspecified int
	for _, c := range candidates {
		switch c.SubmissionKind {
		case "tech_spec":
			techN++
			techSum += c.ConfidenceP
			if c.PatternHint == "manual_review" {
				techManual++
			}
		case "code_snippet":
			codeN++
			codeSum += c.ConfidenceP
			if c.PatternHint == "manual_review" {
				codeManual++
			}
		default:
			unspecified++
		}
	}
	out.TechSpecCount = techN
	out.CodeSnippetCount = codeN
	out.UnspecifiedCount = unspecified
	if techN > 0 {
		out.TechSpecMeanP = floatPtr(techSum / float64(techN))
		out.TechSpecGapRate = floatPtr(float64(techManual) / float64(techN))
	}
	if codeN > 0 {
		out.CodeSnippetMeanP = floatPtr(codeSum / float64(codeN))
		out.CodeSnippetGapRate = floatPtr(float64(codeManual) / float64(codeN))
	}
	denom := candidateTotal
	if denom <= 0 {
		denom = len(candidates)
	}
	if denom > 0 {
		out.UnspecifiedRate = floatPtr(float64(unspecified) / float64(denom))
	}
	return out
}

// EmpiricalPredictionSetCoverage is the share of candidates whose assigned band
// appears in their conformal prediction_set (run-level validity check).
func EmpiricalPredictionSetCoverage(candidates []kindCand) *float64 {
	covered, withSet := 0, 0
	for _, c := range candidates {
		if len(c.PredictionSet) == 0 || c.Confidence == "" {
			continue
		}
		withSet++
		band := strings.ToLower(c.Confidence)
		for _, b := range c.PredictionSet {
			if strings.ToLower(b) == band {
				covered++
				break
			}
		}
	}
	if withSet == 0 {
		return nil
	}
	return floatPtr(float64(covered) / float64(withSet))
}

// CostPerUsefulOutcome returns USD per accepted proposal and per FAIL observation.
func CostPerUsefulOutcome(costUSD float64, acceptedProposals, failCount int) (perAccepted, perFail *float64) {
	if costUSD < 0 {
		return nil, nil
	}
	if acceptedProposals > 0 {
		perAccepted = floatPtr(costUSD / float64(acceptedProposals))
	}
	if failCount > 0 {
		perFail = floatPtr(costUSD / float64(failCount))
	}
	return perAccepted, perFail
}

// PublishFreshnessSLO computes age of a run/publish timestamp vs now.
func PublishFreshnessSLO(timestamp string, now time.Time) (ageMinutes int, ok bool) {
	ts := strings.TrimSpace(timestamp)
	if ts == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// try date-only / without zone variants commonly seen in Actions
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
			if t2, err2 := time.Parse(layout, ts); err2 == nil {
				t = t2
				err = nil
				break
			}
		}
		if err != nil {
			return 0, false
		}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	mins := int(now.Sub(t.UTC()).Minutes())
	if mins < 0 {
		mins = 0
	}
	return mins, mins <= PublishFreshnessMaxAgeMinutes
}

// WilsonRate is a small helper for optional CI emission.
func WilsonRate(successes, total int) (rate, lo, hi *float64) {
	if total <= 0 {
		return nil, nil, nil
	}
	r := float64(successes) / float64(total)
	ci := calibration.WilsonInterval(successes, total, 0.95)
	return floatPtr(r), floatPtr(ci.Low), floatPtr(ci.High)
}
