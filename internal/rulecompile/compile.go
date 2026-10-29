package rulecompile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

// Compile runs extract → LLM draft → gates over an ADR root.
func Compile(opts Options) (Report, error) {
	var zero Report
	if strings.TrimSpace(opts.Root) == "" {
		return zero, fmt.Errorf("--root is required")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return zero, fmt.Errorf("--out is required")
	}
	if opts.Provider == nil {
		opts.Provider = OfflineProvider{}
	}
	rootAbs, err := filepath.Abs(opts.Root)
	if err != nil {
		return zero, err
	}
	outAbs, err := filepath.Abs(opts.OutDir)
	if err != nil {
		return zero, err
	}
	if err := os.MkdirAll(outAbs, 0o750); err != nil {
		return zero, err
	}

	docs, err := adringest.ParseDir(rootAbs)
	if err != nil {
		return zero, fmt.Errorf("ingest ADRs: %w", err)
	}
	ingestPayload := struct {
		SchemaVersion string               `json:"schema_version"`
		SourceRoot    string               `json:"source_root"`
		Documents     []adringest.Document `json:"documents"`
	}{
		SchemaVersion: "1",
		SourceRoot:    filepath.ToSlash(filepath.Clean(rootAbs)),
		Documents:     docs,
	}
	ingestRaw, err := json.Marshal(ingestPayload)
	if err != nil {
		return zero, err
	}
	index, err := obligations.BuildIndex(docs, rootAbs, filepath.ToSlash(filepath.Clean(rootAbs)), ingestRaw)
	if err != nil {
		return zero, fmt.Errorf("extract obligations: %w", err)
	}
	candPath := filepath.Join(outAbs, "candidates.json")
	if err := writeJSON(candPath, index); err != nil {
		return zero, err
	}

	modalities := normalizeModalities(opts.Modalities)
	selected := selectCandidates(index.Candidates, modalities, opts.MinConfidenceP, opts.ADRFilter, opts.Limit)

	report := Report{
		SchemaVersion:     SchemaVersion,
		PromptVersion:     PromptVersion,
		SourceRoot:        filepath.ToSlash(filepath.Clean(rootAbs)),
		OutDir:            filepath.ToSlash(filepath.Clean(outAbs)),
		Provider:          opts.Provider.Name(),
		Model:             opts.Provider.Model(),
		CandidateTotal:    len(index.Candidates),
		CompiledTotal:     len(selected),
		ConfidenceSummary: index.ConfidenceSummary,
		Items:             make([]Item, 0, len(selected)),
		Notes: []string{
			"Drafts are never executable policy. Human review + catalog bind required.",
			"Assess/PR paths remain zero-LLM; this tool is compile-time only.",
		},
	}

	draftsRoot := filepath.Join(outAbs, "drafts")
	if err := os.MkdirAll(draftsRoot, 0o750); err != nil {
		return zero, err
	}

	for _, cand := range selected {
		item, err := compileOne(cand, opts, draftsRoot)
		if err != nil {
			item = Item{
				CandidateID:        cand.ID,
				ADRID:              cand.ADRID,
				Statement:          cand.Statement,
				Modality:           cand.Modality,
				ExtractConfidence:  cand.Confidence,
				ExtractConfidenceP: cand.ConfidenceP,
				Executable:         false,
				Draft: Draft{
					Route:     RouteManualReview,
					Rationale: "compile error: " + err.Error(),
				},
				Gate: GateResult{Status: "failed", Errors: []string{err.Error()}},
			}
			item.CombinedConfidenceP, item.CombinedConfidence = combineConfidence(cand.ConfidenceP, 0, item.Gate)
		}
		report.PromptTokens += item.PromptTokens
		report.CompletionTokens += item.CompletionTokens
		switch item.Draft.Route {
		case RouteBindExisting:
			report.BindExisting++
		case RouteInexpressible:
			report.Inexpressible++
		case RouteManualReview:
			report.ManualReview++
		}
		switch item.Gate.Status {
		case "passed":
			report.GatePassed++
		case "failed":
			report.GateFailed++
		}
		report.Items = append(report.Items, item)
	}

	if err := writeJSON(filepath.Join(outAbs, "report.json"), report); err != nil {
		return zero, err
	}
	if err := os.WriteFile(filepath.Join(outAbs, "report.md"), []byte(RenderMarkdown(report)), 0o600); err != nil {
		return zero, err
	}
	return report, nil
}

func compileOne(cand obligations.Candidate, opts Options, draftsRoot string) (Item, error) {
	item := Item{
		CandidateID:        cand.ID,
		ADRID:              cand.ADRID,
		ADRTitle:           cand.ADRTitle,
		Statement:          cand.Statement,
		Modality:           cand.Modality,
		SourcePath:         cand.SourcePath,
		SourceLineStart:    cand.SourceLineStart,
		SourceLineEnd:      cand.SourceLineEnd,
		ExtractConfidence:  cand.Confidence,
		ExtractConfidenceP: cand.ConfidenceP,
		PatternHint:        cand.PatternHint,
		Executable:         false,
	}

	dirName := sanitizeDir(cand.ADRID + "__" + cand.ID)
	draftDir := filepath.Join(draftsRoot, dirName)
	if err := os.MkdirAll(draftDir, 0o750); err != nil {
		return item, err
	}
	item.DraftDir = filepath.ToSlash(draftDir)

	feedback := ""
	var draft Draft
	var gate GateResult
	attempts := 0
	maxAttempts := 1
	if opts.Repair {
		maxAttempts = MaxRepairTries
	}

	for attempts < maxAttempts {
		attempts++
		prompt := buildUserPrompt(cand, feedback)
		raw, usage, err := opts.Provider.Complete(prompt)
		item.PromptTokens += usage.PromptTokens
		item.CompletionTokens += usage.CompletionTokens
		if err != nil {
			return item, err
		}
		if err := json.Unmarshal(raw, &draft); err != nil {
			return item, fmt.Errorf("decode draft JSON: %w\nraw: %s", err, truncate(string(raw), 400))
		}
		draft = normalizeDraft(draft)
		_ = os.WriteFile(filepath.Join(draftDir, fmt.Sprintf("raw-attempt-%d.json", attempts)), raw, 0o600)

		if opts.SkipGate || draft.Route != RouteCompileNew {
			gate = structuralValidate(draft)
			if draft.Route != RouteCompileNew {
				gate.Status = "not_applicable"
			} else if opts.SkipGate && gate.StructuralOK {
				gate.Status = "skipped"
			}
		} else {
			gate = runOpenGrepGates(opts.OpenGrepBinary, draftDir, draft)
		}

		meta := map[string]any{"draft": draft, "gate": gate, "attempt": attempts}
		_ = writeJSON(filepath.Join(draftDir, "meta.json"), meta)
		if draft.RuleYAML != "" {
			_ = os.WriteFile(filepath.Join(draftDir, "rule.yml"), []byte(draft.RuleYAML), 0o600)
		}

		if draft.Route != RouteCompileNew || gate.Status == "passed" || gate.Status == "skipped" || gate.Status == "not_applicable" {
			break
		}
		feedback = strings.Join(gate.Errors, "\n")
		if strings.TrimSpace(feedback) == "" {
			feedback = "gate status=" + gate.Status
		}
	}

	item.Draft = draft
	item.Gate = gate
	item.RepairAttempts = attempts
	item.CombinedConfidenceP, item.CombinedConfidence = combineConfidence(cand.ConfidenceP, draft.Confidence, gate)
	_ = writeJSON(filepath.Join(draftDir, "gate.json"), gate)
	return item, nil
}

func normalizeDraft(d Draft) Draft {
	d.Route = Route(strings.ToLower(strings.TrimSpace(string(d.Route))))
	switch d.Route {
	case RouteCompileNew, RouteBindExisting, RouteInexpressible, RouteManualReview:
	default:
		if strings.TrimSpace(d.RuleYAML) != "" {
			d.Route = RouteCompileNew
		} else if len(d.ExistingRuleIDs) > 0 {
			d.Route = RouteBindExisting
		} else {
			d.Route = RouteManualReview
		}
	}
	if d.Confidence < 0 {
		d.Confidence = 0
	}
	if d.Confidence > 1 {
		d.Confidence = 1
	}
	if d.RuleID == "" {
		if m := ruleIDPattern.FindStringSubmatch(d.RuleYAML); len(m) == 2 {
			d.RuleID = m[1]
		}
	}
	return d
}

func combineConfidence(extractP, llmP float64, gate GateResult) (float64, string) {
	if extractP < 0 {
		extractP = 0
	}
	if llmP < 0 {
		llmP = 0
	}
	p := 0.55*extractP + 0.45*llmP
	switch gate.Status {
	case "passed":
		if p < 0.85 {
			p = (p + 0.85) / 2
		}
		if p < 0.75 {
			p = 0.75
		}
	case "failed":
		p *= 0.45
	case "unavailable", "skipped":
		p *= 0.85
	case "not_applicable":
		// bind_existing / inexpressible — keep blended score
	}
	if p > 1 {
		p = 1
	}
	band := obligations.ConfidenceLow
	switch {
	case p >= 0.80:
		band = obligations.ConfidenceHigh
	case p >= 0.55:
		band = obligations.ConfidenceMedium
	}
	return p, band
}

func selectCandidates(all []obligations.Candidate, modalities map[string]bool, minP float64, adrFilter string, limit int) []obligations.Candidate {
	out := make([]obligations.Candidate, 0, len(all))
	for _, c := range all {
		if adrFilter != "" && c.ADRID != adrFilter {
			continue
		}
		if !modalities[strings.ToLower(c.Modality)] {
			continue
		}
		if c.ConfidenceP < minP {
			continue
		}
		out = append(out, c)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func normalizeModalities(in []string) map[string]bool {
	out := map[string]bool{}
	if len(in) == 0 {
		out["must"] = true
		out["must_not"] = true
		return out
	}
	for _, m := range in {
		out[strings.ToLower(strings.TrimSpace(m))] = true
	}
	return out
}

func sanitizeDir(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}
