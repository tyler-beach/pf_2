package obligations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/assessment"
)

const (
	ExtractionMethodDeterministic = "deterministic"
	ExtractionMethodLLM           = "llm"

	ExtractLLMPromptVersion = "pathfinder-extract-llm-v1"
	maxExtractLLMContextBytes = 8 << 10 // cheap model + small context
	maxExtractLLMCandidates   = 32
)

// LLMExtractMeta records optional batch LLM scaffolding spend on a candidate index.
// It never feeds assess/PR-scan evidence; workshop rolls tokens into assist/cost KPIs.
type LLMExtractMeta struct {
	Enabled          bool    `json:"enabled,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	Model            string  `json:"model,omitempty"`
	PromptVersion    string  `json:"prompt_version,omitempty"`
	RuntimeLLMCalls  int     `json:"runtime_llm_calls,omitempty"`
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd,omitempty"`
	CostCurrency     string  `json:"cost_currency,omitempty"`
	CostPricingNote  string  `json:"cost_pricing_note,omitempty"`
	LLMCandidateCount int    `json:"llm_candidate_count,omitempty"`
	SubmissionKind   string  `json:"submission_kind,omitempty"`
}

// ExtractLLMOptions configures kind-gated, HITL-only prose→candidate scaffolding.
type ExtractLLMOptions struct {
	SubmissionKind string
	// Force bypasses the kind gate when an operator explicitly opts in
	// (still never marks candidates executable/approved).
	Force         bool
	Provider      AssistProvider
	MaxCandidates int
	MaxDocBytes   int
}

// ExtractLLMEnabledFromEnv is the optional env gate (off by default).
// PATHFINDER_EXTRACT_LLM=1|true|yes enables batch extract-llm when CLI flag is omitted.
func ExtractLLMEnabledFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PATHFINDER_EXTRACT_LLM"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// AllowExtractLLM enforces the kind gate. Pure code_snippet / unspecified
// without Force is refused so repo CI assess never silently turns LLM on.
func AllowExtractLLM(kind string, force bool) error {
	if force {
		return nil
	}
	parsed, err := assessment.ParseSubmissionKind(kind)
	if err != nil {
		return fmt.Errorf("extract-llm kind gate: %w", err)
	}
	if parsed.AllowsLLMExtract() {
		return nil
	}
	if parsed == assessment.SubmissionKindUnspecified {
		return fmt.Errorf("extract-llm kind gate: submission_kind is required (tech_spec or mixed); code_snippet / repo CI stay deterministic-only")
	}
	return fmt.Errorf("extract-llm kind gate: refusing submission_kind %q (allowed: tech_spec, mixed, or --extract-llm-force)", parsed)
}

// HasLLMExtractedCandidates reports whether the index carries llm-tagged rows.
func HasLLMExtractedCandidates(index Index) bool {
	for _, c := range index.Candidates {
		if strings.EqualFold(strings.TrimSpace(c.ExtractionMethod), ExtractionMethodLLM) {
			return true
		}
	}
	return false
}

// DeterministicCandidates returns a copy of the index with llm-tagged rows
// removed. Used by assess-obligations freshness checks so LLM scaffolding
// cannot taint the deterministic rebuild comparison or reviewdog evidence.
func DeterministicCandidates(index Index) Index {
	out := index
	out.LLMExtract = LLMExtractMeta{}
	filtered := make([]Candidate, 0, len(index.Candidates))
	for _, c := range index.Candidates {
		method := strings.TrimSpace(c.ExtractionMethod)
		if strings.EqualFold(method, ExtractionMethodLLM) {
			continue
		}
		filtered = append(filtered, c)
	}
	out.Candidates = filtered
	out.CandidateCount = len(filtered)
	out.ConfidenceSummary = SummarizeConfidence(out)
	docs := map[string]struct{}{}
	for _, c := range filtered {
		docs[c.SourcePath] = struct{}{}
	}
	out.DocumentsWithCandidates = len(docs)
	return out
}

// MergeLLMCandidates unions deterministic extract with LLM proposals.
// Prefer deterministic on statement/path collisions; LLM-only rows stay
// pending_review / non-executable with extraction_method=llm.
func MergeLLMCandidates(deterministic Index, llm []Candidate, meta LLMExtractMeta) Index {
	out := deterministic
	seen := make(map[string]struct{}, len(deterministic.Candidates))
	stamped := make([]Candidate, len(deterministic.Candidates))
	copy(stamped, deterministic.Candidates)
	for i := range stamped {
		if strings.TrimSpace(stamped[i].ExtractionMethod) == "" {
			stamped[i].ExtractionMethod = ExtractionMethodDeterministic
		}
		seen[dedupeKey(stamped[i])] = struct{}{}
	}
	merged := stamped
	llmOnly := 0
	for _, c := range llm {
		key := dedupeKey(c)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		c.ExtractionMethod = ExtractionMethodLLM
		c.ApprovalStatus = DecisionPending
		c.EnforcementMode = EnforcementReviewRequired
		c.Executable = false
		scored := ScoreCandidate(c)
		c.Confidence = scored.ConfidenceBand
		c.ConfidenceP = scored.ConfidenceP
		c.ConfidenceMethod = scored.ConfidenceMethod
		c.ConfidenceCoverage = scored.ConfidenceCoverage
		c.PredictionSet = scored.PredictionSet
		c.EscalationReasons = appendUniqueHint(scored.EscalationReasons, "llm_extract")
		c.ReviewHints = appendUniqueHint(c.ReviewHints, "llm_scaffolding")
		if c.ID == "" {
			c.ID = candidateID(c)
		}
		merged = append(merged, c)
		llmOnly++
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].SourcePath != merged[j].SourcePath {
			return merged[i].SourcePath < merged[j].SourcePath
		}
		if merged[i].SourceLineStart != merged[j].SourceLineStart {
			return merged[i].SourceLineStart < merged[j].SourceLineStart
		}
		return merged[i].ID < merged[j].ID
	})
	out.Candidates = merged
	out.CandidateCount = len(merged)
	out.ConfidenceSummary = SummarizeConfidence(out)
	docs := map[string]struct{}{}
	for _, c := range merged {
		docs[c.SourcePath] = struct{}{}
	}
	out.DocumentsWithCandidates = len(docs)
	meta.Enabled = true
	meta.LLMCandidateCount = llmOnly
	if meta.CostCurrency == "" {
		meta.CostCurrency = "USD"
	}
	if meta.CostPricingNote == "" {
		meta.CostPricingNote = "offline=0; live uses gpt-4o-mini list rates ($0.15/1M input, $0.60/1M output) when usage is reported"
	}
	if meta.PromptVersion == "" {
		meta.PromptVersion = ExtractLLMPromptVersion
	}
	out.LLMExtract = meta
	return out
}

// ExtractLLMProposals asks the provider for schema-constrained candidate JSON
// for each document. Output is untrusted scaffolding — never approved/executable.
func ExtractLLMProposals(ctx context.Context, documents []adringest.Document, sourceRootPath, sourceRootLabel string, opts ExtractLLMOptions) ([]Candidate, LLMExtractMeta, error) {
	meta := LLMExtractMeta{
		PromptVersion:   ExtractLLMPromptVersion,
		SubmissionKind:  strings.TrimSpace(opts.SubmissionKind),
		CostCurrency:    "USD",
		CostPricingNote: "offline=0; live uses gpt-4o-mini list rates ($0.15/1M input, $0.60/1M output) when usage is reported",
	}
	if err := AllowExtractLLM(opts.SubmissionKind, opts.Force); err != nil {
		return nil, meta, err
	}
	if opts.Provider == nil {
		opts.Provider = OfflineExtractLLMProvider{}
	}
	meta.Provider = opts.Provider.Name()
	meta.Model = opts.Provider.Model()
	maxDocs := opts.MaxCandidates
	if maxDocs <= 0 {
		maxDocs = maxExtractLLMCandidates
	}
	maxBytes := opts.MaxDocBytes
	if maxBytes <= 0 {
		maxBytes = maxExtractLLMContextBytes
	}
	kind := strings.TrimSpace(opts.SubmissionKind)
	if kind == "" {
		kind = string(assessment.SubmissionKindTechSpec)
	}
	rootAbs, err := filepath.Abs(sourceRootPath)
	if err != nil {
		return nil, meta, fmt.Errorf("resolve source root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, meta, fmt.Errorf("resolve source root symlinks: %w", err)
	}
	label, err := normalizeSourceRootLabel(sourceRootLabel)
	if err != nil {
		return nil, meta, err
	}
	docs := append([]adringest.Document(nil), documents...)
	sort.Slice(docs, func(i, j int) bool {
		return filepath.ToSlash(docs[i].SourcePath) < filepath.ToSlash(docs[j].SourcePath)
	})
	out := make([]Candidate, 0)
	for _, doc := range docs {
		if len(out) >= maxDocs {
			break
		}
		source, err := readContainedSource(rootReal, doc.SourcePath)
		if err != nil {
			return nil, meta, fmt.Errorf("load source for %s: %w", doc.ID, err)
		}
		displayPath, err := cleanRelativePath(joinSourcePath(label, doc.SourcePath))
		if err != nil {
			return nil, meta, err
		}
		prompt := buildExtractLLMPrompt(doc, source, maxBytes)
		raw, usage, err := opts.Provider.Complete(ctx, prompt)
		if err != nil {
			return nil, meta, fmt.Errorf("extract-llm %s: %w", doc.ID, err)
		}
		meta.RuntimeLLMCalls++
		meta.PromptTokens += usage.PromptTokens
		meta.CompletionTokens += usage.CompletionTokens
		payload, err := parseExtractLLMPayload(raw)
		if err != nil {
			return nil, meta, fmt.Errorf("extract-llm %s: %w", doc.ID, err)
		}
		for _, row := range payload.Candidates {
			if len(out) >= maxDocs {
				break
			}
			cand, err := mapExtractLLMRow(doc, displayPath, source, kind, row)
			if err != nil {
				return nil, meta, fmt.Errorf("extract-llm %s: %w", doc.ID, err)
			}
			out = append(out, cand)
		}
	}
	if meta.Provider == "openai-compatible" {
		meta.EstimatedCostUSD = (float64(meta.PromptTokens)*0.15 + float64(meta.CompletionTokens)*0.60) / 1_000_000
	}
	return out, meta, nil
}

// BuildIndexWithOptionalLLM runs deterministic extract, then optionally merges
// kind-gated LLM scaffolding. Deterministic extract always runs.
func BuildIndexWithOptionalLLM(ctx context.Context, documents []adringest.Document, sourceRootPath, sourceRootLabel string, sourceIndexRaw []byte, enable bool, opts ExtractLLMOptions) (Index, error) {
	index, err := BuildIndex(documents, sourceRootPath, sourceRootLabel, sourceIndexRaw)
	if err != nil {
		return index, err
	}
	if !enable {
		return index, nil
	}
	llmRows, meta, err := ExtractLLMProposals(ctx, documents, sourceRootPath, sourceRootLabel, opts)
	if err != nil {
		return index, err
	}
	return MergeLLMCandidates(index, llmRows, meta), nil
}

// OfflineExtractLLMProvider returns deterministic schema-valid JSON for CI/tests.
type OfflineExtractLLMProvider struct {
	ModelID string
	Fixture []byte // optional full JSON override
}

func (p OfflineExtractLLMProvider) Name() string { return "offline-fixture" }
func (p OfflineExtractLLMProvider) Model() string {
	if strings.TrimSpace(p.ModelID) == "" {
		return "offline-extract-replay"
	}
	return p.ModelID
}

func (p OfflineExtractLLMProvider) Complete(ctx context.Context, prompt string) ([]byte, TokenUsage, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, TokenUsage{}, ctx.Err()
	}
	if len(p.Fixture) > 0 {
		return append([]byte(nil), p.Fixture...), TokenUsage{}, nil
	}
	// Derive a stable synthetic candidate from the prompt so offline CI is
	// schema-valid without network calls. Not a live model judgment.
	sum := sha256.Sum256([]byte(prompt))
	stmt := "Services must apply a finite timeout on outbound calls (offline extract-llm fixture)."
	if quote := extractPromptQuote(prompt); quote != "" {
		stmt = quote
	}
	payload := extractLLMPayload{
		Candidates: []extractLLMRow{{
			Statement:      stmt,
			Modality:       "must",
			PatternHint:    "configuration.bounds_or_required_values",
			SectionHeading: "Decision",
			Conditions:     []string{"outbound_calls"},
			Exceptions:     []string{},
			Ambiguity:      []string{"offline_fixture_replay"},
			SourceLineStart: 1,
			SourceLineEnd:   1,
			FixtureDigest:  hex.EncodeToString(sum[:8]),
		}},
	}
	raw, err := json.Marshal(payload)
	return raw, TokenUsage{}, err
}

type extractLLMPayload struct {
	Candidates []extractLLMRow `json:"candidates"`
}

type extractLLMRow struct {
	Statement       string   `json:"statement"`
	Modality        string   `json:"modality"`
	PatternHint     string   `json:"pattern_hint"`
	SectionHeading  string   `json:"section_heading"`
	Conditions      []string `json:"conditions"`
	Exceptions      []string `json:"exceptions"`
	Ambiguity       []string `json:"ambiguity"`
	SourceLineStart int      `json:"source_line_start"`
	SourceLineEnd   int      `json:"source_line_end"`
	FixtureDigest   string   `json:"fixture_digest,omitempty"`
}

func parseExtractLLMPayload(raw []byte) (extractLLMPayload, error) {
	var payload extractLLMPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		// Offline fixture may include fixture_digest; tolerate unknown fields.
		if err := json.Unmarshal(raw, &payload); err != nil {
			return payload, fmt.Errorf("decode extract-llm JSON: %w", err)
		}
	}
	if payload.Candidates == nil {
		payload.Candidates = []extractLLMRow{}
	}
	if len(payload.Candidates) > maxExtractLLMCandidates {
		return payload, fmt.Errorf("extract-llm returned %d candidates (max %d)", len(payload.Candidates), maxExtractLLMCandidates)
	}
	return payload, nil
}

func mapExtractLLMRow(doc adringest.Document, displayPath string, source []byte, kind string, row extractLLMRow) (Candidate, error) {
	statement := strings.TrimSpace(row.Statement)
	if statement == "" {
		return Candidate{}, fmt.Errorf("candidate statement is required")
	}
	if len(statement) > maxCandidateLineBytes {
		return Candidate{}, fmt.Errorf("candidate statement exceeds %d-byte limit", maxCandidateLineBytes)
	}
	modality := strings.TrimSpace(row.Modality)
	switch modality {
	case "must", "must_not", "should", "may", "not_required":
	default:
		return Candidate{}, fmt.Errorf("unsupported modality %q", modality)
	}
	pattern := strings.TrimSpace(row.PatternHint)
	if pattern == "" {
		pattern = suggestPattern(statement)
	}
	if !validPattern(pattern) {
		return Candidate{}, fmt.Errorf("unsupported pattern_hint %q", pattern)
	}
	section := strings.TrimSpace(row.SectionHeading)
	if section == "" {
		section = "Decision"
	}
	start, end := row.SourceLineStart, row.SourceLineEnd
	lines := strings.Split(string(source), "\n")
	if start < 1 || end < start {
		start, end = 1, 1
	}
	if end > len(lines) {
		end = len(lines)
		if start > end {
			start = end
		}
	}
	// Prefer citing a line that contains the statement when present.
	if loc := findStatementLine(lines, statement); loc > 0 {
		start, end = loc, loc
	}
	kindLabel := "normative_candidate"
	if anyPhrase(statement, "unless", "except", "only when", "other than", "waiver", "exception") {
		kindLabel = "exception_candidate"
	}
	conditions := append([]string(nil), row.Conditions...)
	exceptions := append([]string(nil), row.Exceptions...)
	ambiguity := append([]string(nil), row.Ambiguity...)
	if len(conditions) == 0 && len(exceptions) == 0 && len(ambiguity) == 0 {
		conditions, exceptions, ambiguity = classifyStatementStructure(statement)
	}
	hints := reviewHints(statement, kindLabel, section)
	hints = appendUniqueHint(hints, "llm_scaffolding")
	cand := Candidate{
		ADRID: doc.ID, ADRTitle: doc.Title, ADRStatus: doc.Status,
		SectionHeading: section, Statement: statement, Modality: modality,
		Kind: kindLabel, SubmissionKind: kind, PatternHint: pattern,
		SourcePath: displayPath, SourceSHA256: doc.SourceSHA256,
		SourceLineStart: start, SourceLineEnd: end,
		ApprovalStatus: DecisionPending, EnforcementMode: EnforcementReviewRequired, Executable: false,
		ExtractionMethod: ExtractionMethodLLM,
		ReviewHints:      hints, Conditions: conditions, Exceptions: exceptions, Ambiguity: ambiguity,
	}
	cand.ID = candidateID(cand)
	return cand, nil
}

func buildExtractLLMPrompt(doc adringest.Document, source []byte, maxBytes int) string {
	body := string(source)
	if len(body) > maxBytes {
		body = body[:maxBytes] + "\n…[truncated for extract-llm context budget]"
	}
	return strings.Join([]string{
		"Extract review-only obligation candidates from the ADR/tech-spec prose below.",
		"Reply with JSON only: {\"candidates\":[{\"statement\",\"modality\",\"pattern_hint\",\"section_heading\",\"conditions\",\"exceptions\",\"ambiguity\",\"source_line_start\",\"source_line_end\"}]}.",
		"modality must be one of must|must_not|should|may|not_required.",
		"pattern_hint must be a known Pathfinder pattern id (or manual_review).",
		"Never mark candidates approved or executable. Propose only; humans approve later.",
		"ADR ID: " + doc.ID,
		"Title: " + doc.Title,
		"Source path: " + doc.SourcePath,
		"Source SHA-256: " + doc.SourceSHA256,
		"---",
		body,
	}, "\n")
}

func dedupeKey(c Candidate) string {
	stmt := strings.ToLower(strings.Join(strings.Fields(c.Statement), " "))
	path := filepath.ToSlash(strings.TrimSpace(c.SourcePath))
	return path + "\x00" + stmt
}

func findStatementLine(lines []string, statement string) int {
	want := compactSpaces(statement)
	if want == "" {
		return 0
	}
	for i, line := range lines {
		if strings.Contains(compactSpaces(line), want) {
			return i + 1
		}
	}
	return 0
}

func compactSpaces(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func extractPromptQuote(prompt string) string {
	const marker = "---\n"
	idx := strings.LastIndex(prompt, marker)
	if idx < 0 {
		return ""
	}
	body := strings.TrimSpace(prompt[idx+len(marker):])
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "<!--") {
			continue
		}
		if _, ok := detectModality(trim); ok {
			return normalizeStatement(trim)
		}
	}
	return ""
}

// AssertExtractLLMHITL fails closed if any LLM-tagged candidate looks approved
// or executable (propose ≠ approve).
func AssertExtractLLMHITL(index Index) error {
	for _, c := range index.Candidates {
		if !strings.EqualFold(strings.TrimSpace(c.ExtractionMethod), ExtractionMethodLLM) {
			continue
		}
		if c.Executable {
			return fmt.Errorf("extract-llm HITL: candidate %s marked executable; LLM extract must never auto-approve", c.ID)
		}
		if c.ApprovalStatus != "" && c.ApprovalStatus != DecisionPending {
			return fmt.Errorf("extract-llm HITL: candidate %s approval_status=%q; must remain pending_review", c.ID, c.ApprovalStatus)
		}
	}
	return nil
}
