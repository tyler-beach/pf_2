package rulecompile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tyler-beach/pf_2/internal/obligations"
)

// OfflineProvider returns deterministic drafts without network calls.
// Used for CI and local dry-runs; still goes through OpenGrep gates when enabled.
type OfflineProvider struct {
	ModelID string
}

func (p OfflineProvider) Name() string { return "offline" }
func (p OfflineProvider) Model() string {
	if strings.TrimSpace(p.ModelID) == "" {
		return "offline-fixture"
	}
	return p.ModelID
}

func (p OfflineProvider) Complete(prompt string) ([]byte, obligations.TokenUsage, error) {
	// Extremely small heuristic: produce a Python eval-ban style rule that
	// exercises the gate path. Real corpus work uses Anthropic.
	_ = prompt
	draft := Draft{
		Route:      RouteCompileNew,
		Confidence: 0.55,
		Rationale:  "offline fixture draft for gate plumbing; replace with Anthropic for real compiles",
		PatternID:  "security.source_pattern",
		Languages:  []string{"python"},
		RuleID:     "pathfinder.compiled.offline-eval",
		RuleYAML: `rules:
  - id: pathfinder.compiled.offline-eval
    message: "Avoid eval (offline compile fixture)"
    languages: [python]
    severity: ERROR
    pattern: eval(...)
`,
		BadFilename:  "bad.py",
		BadContent:   "def run(x):\n    return eval(x)\n",
		GoodFilename: "good.py",
		GoodContent:  "def run(x):\n    return str(x)\n",
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	return raw, obligations.TokenUsage{PromptTokens: 0, CompletionTokens: 0}, nil
}

// AnthropicProvider calls Anthropic Messages API (api.anthropic.com).
type AnthropicProvider struct {
	APIKey     string
	ModelID    string
	HTTPClient *http.Client
	MaxTokens  int
	Endpoint   string
}

func (p AnthropicProvider) Name() string { return "anthropic" }
func (p AnthropicProvider) Model() string {
	if strings.TrimSpace(p.ModelID) == "" {
		return DefaultModel
	}
	return p.ModelID
}

func (p AnthropicProvider) Complete(prompt string) ([]byte, obligations.TokenUsage, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, obligations.TokenUsage{}, fmt.Errorf("ANTHROPIC_API_KEY (or --api-key-env) is required for anthropic provider")
	}
	endpoint := strings.TrimSpace(p.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}
	maxTokens := p.MaxTokens
	if maxTokens < 1 {
		maxTokens = 4096
	}
	body := map[string]any{
		"model":      p.Model(),
		"max_tokens": maxTokens,
		"temperature": 0,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, obligations.TokenUsage{}, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, truncate(string(raw), 800))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, obligations.TokenUsage{}, fmt.Errorf("decode anthropic response: %w", err)
	}
	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	content := stripJSONFence(text.String())
	if content == "" {
		return nil, obligations.TokenUsage{}, fmt.Errorf("anthropic returned empty content")
	}
	usage := obligations.TokenUsage{
		PromptTokens:     parsed.Usage.InputTokens,
		CompletionTokens: parsed.Usage.OutputTokens,
	}
	return []byte(content), usage, nil
}

// OpenAICompatProvider mirrors obligations.OpenAICompatProvider for non-Anthropic endpoints.
type OpenAICompatProvider struct {
	Endpoint   string
	APIKey     string
	ModelID    string
	HTTPClient *http.Client
	MaxTokens  int
}

func (p OpenAICompatProvider) Name() string  { return "openai-compatible" }
func (p OpenAICompatProvider) Model() string { return p.ModelID }

func (p OpenAICompatProvider) Complete(prompt string) ([]byte, obligations.TokenUsage, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, obligations.TokenUsage{}, fmt.Errorf("API key required for openai-compatible provider")
	}
	if strings.TrimSpace(p.ModelID) == "" {
		return nil, obligations.TokenUsage{}, fmt.Errorf("model required for openai-compatible provider")
	}
	endpoint := strings.TrimSpace(p.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	maxTokens := p.MaxTokens
	if maxTokens < 1 {
		maxTokens = 4096
	}
	body := map[string]any{
		"model": p.ModelID,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": 0,
		"max_tokens":  maxTokens,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, obligations.TokenUsage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, obligations.TokenUsage{}, fmt.Errorf("openai-compatible HTTP %d: %s", resp.StatusCode, truncate(string(raw), 800))
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
		return nil, obligations.TokenUsage{}, err
	}
	if len(parsed.Choices) == 0 {
		return nil, obligations.TokenUsage{}, fmt.Errorf("openai-compatible returned no choices")
	}
	content := stripJSONFence(parsed.Choices[0].Message.Content)
	usage := obligations.TokenUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
	}
	return []byte(content), usage, nil
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```JSON")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
