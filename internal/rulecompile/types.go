// Package rulecompile turns extracted ADR obligations into draft OpenGrep rules.
// LLM calls are batch/compile-time only. Drafts are never executable policy;
// OpenGrep fixture gates and human review are required before any catalog bind.
package rulecompile

import "github.com/tyler-beach/pf_2/internal/obligations"

const (
	SchemaVersion   = "1"
	PromptVersion   = "pathfinder-rulecompile-v1"
	DefaultModel    = "claude-sonnet-4-5"
	MaxRepairTries  = 3
)

// Route is the compiler's disposition for one obligation.
type Route string

const (
	RouteCompileNew    Route = "compile_new"
	RouteBindExisting  Route = "bind_existing"
	RouteInexpressible Route = "inexpressible"
	RouteManualReview  Route = "manual_review"
)

// Provider is a compile-time LLM. OfflineProvider never uses the network.
type Provider interface {
	Name() string
	Model() string
	Complete(prompt string) (content []byte, usage obligations.TokenUsage, err error)
}

// Options configures a corpus compile run.
type Options struct {
	Root           string
	OutDir         string
	Provider       Provider
	OpenGrepBinary string
	Limit          int
	Modalities     []string // empty => must, must_not
	MinConfidenceP float64
	ADRFilter      string // optional exact ADR id
	SkipGate       bool   // structural validation only (no OpenGrep)
	Repair         bool   // retry compile with gate errors
}

// Draft is the structured LLM (or offline) compile artifact for one candidate.
type Draft struct {
	Route           Route    `json:"route"`
	Confidence      float64  `json:"llm_confidence"`
	Rationale       string   `json:"rationale"`
	PatternID       string   `json:"pattern_id,omitempty"`
	ExistingRuleIDs []string `json:"existing_rule_ids,omitempty"`
	Languages       []string `json:"languages,omitempty"`
	RuleYAML        string   `json:"rule_yaml,omitempty"`
	RuleID          string   `json:"rule_id,omitempty"`
	BadFilename     string   `json:"bad_filename,omitempty"`
	BadContent      string   `json:"bad_content,omitempty"`
	GoodFilename    string   `json:"good_filename,omitempty"`
	GoodContent     string   `json:"good_content,omitempty"`
}

// GateResult is the deterministic OpenGrep (or structural) check outcome.
type GateResult struct {
	Status           string   `json:"status"` // passed|failed|skipped|unavailable|not_applicable
	BadFindings      int      `json:"bad_findings"`
	GoodFindings     int      `json:"good_findings"`
	RuleID           string   `json:"rule_id,omitempty"`
	Errors           []string `json:"errors,omitempty"`
	OpenGrepBinary   string   `json:"opengrep_binary,omitempty"`
	OpenGrepVersion  string   `json:"opengrep_version,omitempty"`
	StructuralOK     bool     `json:"structural_ok"`
}

// Item is one obligation's compile result.
type Item struct {
	CandidateID          string   `json:"candidate_id"`
	ADRID                string   `json:"adr_id"`
	ADRTitle             string   `json:"adr_title"`
	Statement            string   `json:"statement"`
	Modality             string   `json:"modality"`
	SourcePath           string   `json:"source_path"`
	SourceLineStart      int      `json:"source_line_start"`
	SourceLineEnd        int      `json:"source_line_end"`
	ExtractConfidence    string   `json:"extract_confidence"`
	ExtractConfidenceP   float64  `json:"extract_confidence_p"`
	PatternHint          string   `json:"pattern_hint"`
	Draft                Draft    `json:"draft"`
	Gate                 GateResult `json:"gate"`
	CombinedConfidenceP  float64  `json:"combined_confidence_p"`
	CombinedConfidence   string   `json:"combined_confidence"`
	Executable           bool     `json:"executable"` // always false
	DraftDir             string   `json:"draft_dir,omitempty"`
	RepairAttempts       int      `json:"repair_attempts,omitempty"`
	PromptTokens         int      `json:"prompt_tokens,omitempty"`
	CompletionTokens     int      `json:"completion_tokens,omitempty"`
}

// Report is the top-level compile run artifact.
type Report struct {
	SchemaVersion      string                     `json:"schema_version"`
	PromptVersion      string                     `json:"prompt_version"`
	SourceRoot         string                     `json:"source_root"`
	OutDir             string                     `json:"out_dir"`
	Provider           string                     `json:"provider"`
	Model              string                     `json:"model"`
	CandidateTotal     int                        `json:"candidate_total"`
	CompiledTotal      int                        `json:"compiled_total"`
	GatePassed         int                        `json:"gate_passed"`
	GateFailed         int                        `json:"gate_failed"`
	BindExisting       int                        `json:"bind_existing"`
	Inexpressible      int                        `json:"inexpressible"`
	ManualReview       int                        `json:"manual_review"`
	PromptTokens       int                        `json:"prompt_tokens"`
	CompletionTokens   int                        `json:"completion_tokens"`
	ConfidenceSummary  obligations.ConfidenceSummary `json:"confidence_summary"`
	Items              []Item                     `json:"items"`
	Notes              []string                   `json:"notes"`
}
