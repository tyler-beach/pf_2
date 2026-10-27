package obligations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ProposalSchemaVersion = "1"
const AssistPromptVersion = "pathfinder-assist-v1"

// Proposal is an untrusted LLM (or offline fixture) suggestion. It never
// approves obligations and never mutates the requirements catalog.
type Proposal struct {
	SchemaVersion       string   `json:"schema_version"`
	ProposalID          string   `json:"proposal_id"`
	ParentCandidateID   string   `json:"parent_candidate_id"`
	ADRID               string   `json:"adr_id"`
	Statement           string   `json:"statement"`
	NormalizedStatement string   `json:"normalized_statement"`
	Modality            string   `json:"modality"`
	SuggestedPatternID  string   `json:"suggested_pattern_id"`
	Rationale           string   `json:"rationale"`
	SourcePath          string   `json:"source_path"`
	SourceSHA256        string   `json:"source_sha256"`
	SourceLineStart     int      `json:"source_line_start"`
	SourceLineEnd       int      `json:"source_line_end"`
	SourceQuote         string   `json:"source_quote"`
	Confidence          string   `json:"parent_confidence"`
	EscalationReasons   []string `json:"escalation_reasons,omitempty"`
	Provider            string   `json:"provider"`
	Model               string   `json:"model"`
	PromptVersion       string   `json:"prompt_version"`
	PromptSHA256        string   `json:"prompt_sha256"`
	RawResponseSHA256   string   `json:"raw_response_sha256"`
	ApprovalStatus      string   `json:"approval_status"`
	Executable          bool     `json:"executable"`
	GeneratedAt         string   `json:"generated_at"`
}

// ProposalIndex is the assist artifact written for human review.
type ProposalIndex struct {
	SchemaVersion     string     `json:"schema_version"`
	SourceIndexSHA256 string     `json:"source_index_sha256"`
	MaxConfidence     string     `json:"max_confidence"`
	Provider          string     `json:"provider"`
	Model             string     `json:"model"`
	PromptVersion     string     `json:"prompt_version"`
	CandidateCount    int        `json:"candidate_count"`
	ProposalCount     int        `json:"proposal_count"`
	RuntimeLLMCalls   int        `json:"runtime_llm_calls"`
	PromptTokens      int        `json:"prompt_tokens"`
	CompletionTokens  int        `json:"completion_tokens"`
	EstimatedCostUSD  float64    `json:"estimated_cost_usd"`
	CostCurrency      string     `json:"cost_currency,omitempty"`
	CostPricingNote   string     `json:"cost_pricing_note,omitempty"`
	Proposals         []Proposal `json:"proposals"`
}

// TokenUsage is optional provider token accounting for cost rollups.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
}

// AssistProvider generates raw proposal JSON for one candidate. Live providers
// may call external APIs; OfflineAssistProvider never does.
type AssistProvider interface {
	Name() string
	Model() string
	Complete(ctx context.Context, prompt string) (raw []byte, err error)
}

// OfflineAssistProvider produces deterministic proposals without network calls.
// Used for default CI / dry-run / Promptfoo fixture replay.
type OfflineAssistProvider struct {
	ModelID string
}

func (p OfflineAssistProvider) Name() string { return "offline-fixture" }
func (p OfflineAssistProvider) Model() string {
	if strings.TrimSpace(p.ModelID) == "" {
		return "offline-replay"
	}
	return p.ModelID
}

func (p OfflineAssistProvider) Complete(ctx context.Context, prompt string) ([]byte, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// Deterministic structured stub derived from the prompt hash; not a model.
	sum := sha256.Sum256([]byte(prompt))
	payload := map[string]any{
		"normalized_statement": "offline assist replay — review required",
		"modality":             "must",
		"suggested_pattern_id": "manual_review",
		"rationale":            "Deterministic offline fixture response for Promptfoo/CI; not a live model judgment.",
		"fixture_digest":       hex.EncodeToString(sum[:8]),
	}
	return json.Marshal(payload)
}

// OpenAICompatProvider calls an OpenAI-compatible chat completions endpoint.
type OpenAICompatProvider struct {
	Endpoint   string
	APIKey     string
	ModelID    string
	HTTPClient *http.Client
	MaxTokens  int
	// LastUsage is populated after each successful Complete call.
	LastUsage TokenUsage
}

func (p OpenAICompatProvider) Name() string  { return "openai-compatible" }
func (p OpenAICompatProvider) Model() string { return p.ModelID }

func (p OpenAICompatProvider) Complete(ctx context.Context, prompt string) ([]byte, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, fmt.Errorf("API key is required for live LLM assist")
	}
	if strings.TrimSpace(p.ModelID) == "" {
		return nil, fmt.Errorf("model is required for live LLM assist")
	}
	endpoint := strings.TrimSpace(p.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	maxTokens := p.MaxTokens
	if maxTokens < 1 {
		maxTokens = 512
	}
	body := map[string]any{
		"model": p.ModelID,
		"messages": []map[string]string{
			{"role": "system", "content": "You assist Pathfinder ADR obligation review. Reply with JSON only containing normalized_statement, modality, suggested_pattern_id, rationale. Never mark obligations approved or executable."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
		"max_tokens":  maxTokens,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LLM provider returned HTTP %d: %s", resp.StatusCode, truncateAssist(string(raw), 512))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode provider response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("provider returned no assistant content")
	}
	p.LastUsage = TokenUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return []byte(strings.TrimSpace(content)), nil
}

// AssistOptions configures confidence-gated proposal generation.
type AssistOptions struct {
	MaxConfidence string
	MaxCandidates int
	SourceRoot    string // directory containing candidate SourcePath files; required for citation checks
	Provider      AssistProvider
	Now           func() time.Time
}

// AssistObligations generates review-only proposals for candidates at or below
// MaxConfidence. It never writes requirements.json or the review registry.
func AssistObligations(ctx context.Context, index Index, opts AssistOptions) (ProposalIndex, error) {
	out := ProposalIndex{
		SchemaVersion:     ProposalSchemaVersion,
		SourceIndexSHA256: index.SourceIndexSHA256,
		MaxConfidence:     strings.ToLower(strings.TrimSpace(opts.MaxConfidence)),
		PromptVersion:     AssistPromptVersion,
		Proposals:         []Proposal{},
	}
	if out.MaxConfidence == "" {
		out.MaxConfidence = ConfidenceLow
	}
	if opts.Provider == nil {
		opts.Provider = OfflineAssistProvider{}
	}
	out.Provider = opts.Provider.Name()
	out.Model = opts.Provider.Model()
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	selected := FilterByMaxConfidence(index, out.MaxConfidence)
	if opts.MaxCandidates > 0 && len(selected) > opts.MaxCandidates {
		selected = selected[:opts.MaxCandidates]
	}
	out.CandidateCount = len(selected)
	out.CostCurrency = "USD"
	out.CostPricingNote = "offline=0; live uses gpt-4o-mini list rates ($0.15/1M input, $0.60/1M output) when usage is reported"
	for _, candidate := range selected {
		if candidate.Executable {
			return out, fmt.Errorf("refusing to assist executable candidate %s", candidate.ID)
		}
		prompt := buildAssistPrompt(candidate)
		promptSum := sha256.Sum256([]byte(prompt))
		raw, err := opts.Provider.Complete(ctx, prompt)
		if err != nil {
			return out, fmt.Errorf("assist candidate %s: %w", candidate.ID, err)
		}
		out.RuntimeLLMCalls++
		if live, ok := opts.Provider.(*OpenAICompatProvider); ok {
			out.PromptTokens += live.LastUsage.PromptTokens
			out.CompletionTokens += live.LastUsage.CompletionTokens
		}
		rawSum := sha256.Sum256(raw)
		parsed, err := parseAssistPayload(raw)
		if err != nil {
			return out, fmt.Errorf("assist candidate %s: %w", candidate.ID, err)
		}
		quote := candidate.Statement
		if err := verifyAssistCitation(opts.SourceRoot, candidate, quote); err != nil {
			return out, fmt.Errorf("assist candidate %s: %w", candidate.ID, err)
		}
		proposal := Proposal{
			SchemaVersion:       ProposalSchemaVersion,
			ParentCandidateID:   candidate.ID,
			ADRID:               candidate.ADRID,
			Statement:           candidate.Statement,
			NormalizedStatement: parsed.NormalizedStatement,
			Modality:            parsed.Modality,
			SuggestedPatternID:  parsed.SuggestedPatternID,
			Rationale:           parsed.Rationale,
			SourcePath:          candidate.SourcePath,
			SourceSHA256:        candidate.SourceSHA256,
			SourceLineStart:     candidate.SourceLineStart,
			SourceLineEnd:       candidate.SourceLineEnd,
			SourceQuote:         quote,
			Confidence:          candidate.Confidence,
			EscalationReasons:   append([]string(nil), candidate.EscalationReasons...),
			Provider:            opts.Provider.Name(),
			Model:               opts.Provider.Model(),
			PromptVersion:       AssistPromptVersion,
			PromptSHA256:        hex.EncodeToString(promptSum[:]),
			RawResponseSHA256:   hex.EncodeToString(rawSum[:]),
			ApprovalStatus:      DecisionPending,
			Executable:          false,
			GeneratedAt:         nowFn().UTC().Format(time.RFC3339),
		}
		proposal.ProposalID = proposalID(proposal)
		out.Proposals = append(out.Proposals, proposal)
	}
	out.ProposalCount = len(out.Proposals)
	if out.Provider == "openai-compatible" {
		// Approximate list pricing for gpt-4o-mini-class models; workshop surfaces the estimate.
		out.EstimatedCostUSD = (float64(out.PromptTokens)*0.15 + float64(out.CompletionTokens)*0.60) / 1_000_000
	}
	return out, nil
}

type assistPayload struct {
	NormalizedStatement string `json:"normalized_statement"`
	Modality            string `json:"modality"`
	SuggestedPatternID  string `json:"suggested_pattern_id"`
	Rationale           string `json:"rationale"`
}

func parseAssistPayload(raw []byte) (assistPayload, error) {
	var payload assistPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		// Offline fixture may include fixture_digest; tolerate unknown fields.
		if err := json.Unmarshal(raw, &payload); err != nil {
			return payload, fmt.Errorf("decode assist JSON: %w", err)
		}
	}
	payload.NormalizedStatement = strings.TrimSpace(payload.NormalizedStatement)
	payload.Modality = strings.TrimSpace(payload.Modality)
	payload.SuggestedPatternID = strings.TrimSpace(payload.SuggestedPatternID)
	payload.Rationale = strings.TrimSpace(payload.Rationale)
	if payload.NormalizedStatement == "" || payload.Modality == "" || payload.SuggestedPatternID == "" || payload.Rationale == "" {
		return payload, fmt.Errorf("assist JSON missing required fields")
	}
	switch payload.Modality {
	case "must", "must_not", "should", "may", "not_required":
	default:
		return payload, fmt.Errorf("unsupported assist modality %q", payload.Modality)
	}
	if !validPattern(payload.SuggestedPatternID) {
		return payload, fmt.Errorf("unsupported suggested_pattern_id %q", payload.SuggestedPatternID)
	}
	return payload, nil
}

func verifyAssistCitation(sourceRoot string, candidate Candidate, quote string) error {
	if strings.TrimSpace(quote) == "" {
		return fmt.Errorf("empty source quote")
	}
	if quote != candidate.Statement {
		return fmt.Errorf("source quote does not match parent candidate statement")
	}
	if candidate.SourceLineStart < 1 || candidate.SourceLineEnd < candidate.SourceLineStart {
		return fmt.Errorf("invalid source line range")
	}
	if !validSHA256(candidate.SourceSHA256) {
		return fmt.Errorf("invalid source SHA-256")
	}
	root := strings.TrimSpace(sourceRoot)
	if root == "" {
		return fmt.Errorf("source root is required to verify citations against source bytes")
	}
	cleanPath := filepath.Clean(candidate.SourcePath)
	if cleanPath == "." || cleanPath == "" || filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, "..") {
		return fmt.Errorf("unsafe source path %q", candidate.SourcePath)
	}
	fullPath := filepath.Join(root, cleanPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, candidate.SourceSHA256) {
		return fmt.Errorf("source SHA-256 mismatch for %s", candidate.SourcePath)
	}
	lines := strings.Split(string(data), "\n")
	if candidate.SourceLineEnd > len(lines) {
		return fmt.Errorf("source line range %d-%d exceeds file length %d", candidate.SourceLineStart, candidate.SourceLineEnd, len(lines))
	}
	var excerpt strings.Builder
	for i := candidate.SourceLineStart - 1; i < candidate.SourceLineEnd; i++ {
		if i > candidate.SourceLineStart-1 {
			excerpt.WriteByte('\n')
		}
		excerpt.WriteString(lines[i])
	}
	extracted := strings.TrimSpace(excerpt.String())
	want := strings.TrimSpace(quote)
	if extracted != want && !strings.Contains(extracted, want) {
		return fmt.Errorf("source quote not found in cited line range")
	}
	return nil
}

func buildAssistPrompt(candidate Candidate) string {
	return strings.Join([]string{
		"Parent candidate ID: " + candidate.ID,
		"ADR: " + candidate.ADRID,
		"Source path: " + candidate.SourcePath,
		fmt.Sprintf("Source lines: %d-%d", candidate.SourceLineStart, candidate.SourceLineEnd),
		"Source SHA-256: " + candidate.SourceSHA256,
		"Confidence: " + candidate.Confidence,
		"Escalation reasons: " + strings.Join(candidate.EscalationReasons, ", "),
		"Statement quote (must be preserved): " + candidate.Statement,
		"Return JSON with normalized_statement, modality, suggested_pattern_id, rationale.",
	}, "\n")
}

func proposalID(p Proposal) string {
	material := strings.Join([]string{
		p.ParentCandidateID, p.SourceSHA256, fmt.Sprintf("%d", p.SourceLineStart),
		p.PromptSHA256, p.RawResponseSHA256, p.NormalizedStatement,
	}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return "PROP-" + hex.EncodeToString(sum[:8])
}

func truncateAssist(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

// LoadProposalIndex loads an assist artifact.
func LoadProposalIndex(path string) (ProposalIndex, error) {
	var index ProposalIndex
	data, err := os.ReadFile(path)
	if err != nil {
		return index, fmt.Errorf("read proposal index: %w", err)
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return index, fmt.Errorf("decode proposal index: %w", err)
	}
	for _, p := range index.Proposals {
		if p.Executable || p.ApprovalStatus != DecisionPending {
			return index, fmt.Errorf("proposal %s must remain pending_review and non-executable", p.ProposalID)
		}
	}
	return index, nil
}

// MarkdownProposalReport renders assist proposals for human review.
func MarkdownProposalReport(index ProposalIndex) string {
	var b strings.Builder
	b.WriteString("# LLM obligation assist proposals\n\n")
	b.WriteString("> Proposals only: not approvals, not executable, and not catalog mutations.\n\n")
	fmt.Fprintf(&b, "- Provider: `%s` / model `%s`\n", index.Provider, index.Model)
	fmt.Fprintf(&b, "- Max confidence escalated: `%s`\n", index.MaxConfidence)
	fmt.Fprintf(&b, "- Proposals: %d (from %d candidates); runtime LLM calls recorded on this artifact: %d\n\n",
		index.ProposalCount, index.CandidateCount, index.RuntimeLLMCalls)
	if len(index.Proposals) == 0 {
		b.WriteString("No candidates matched the escalation threshold.\n")
		return b.String()
	}
	b.WriteString("| Proposal | Parent | Confidence | Pattern | Statement | Rationale |\n|---|---|---|---|---|---|\n")
	for _, p := range index.Proposals {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n",
			mdCell(p.ProposalID), mdCell(p.ParentCandidateID), mdCell(p.Confidence),
			mdCell(p.SuggestedPatternID), mdCell(p.NormalizedStatement), mdCell(p.Rationale))
	}
	return b.String()
}
