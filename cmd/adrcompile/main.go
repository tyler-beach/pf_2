// Command adrcompile extracts RFC 2119 obligations from a MADR/ADR corpus and
// compiles draft OpenGrep rules with optional Anthropic assistance and
// deterministic fixture gates. Drafts are never executable Pathfinder policy.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tyler-beach/pf_2/internal/rulecompile"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "adrcompile: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("adrcompile", flag.ContinueOnError)
	root := fs.String("root", "", "ADR markdown directory (e.g. testdata/corpus/adrs)")
	out := fs.String("out", "adrcompile-out", "output directory for candidates, drafts, report")
	providerName := fs.String("provider", "offline", "offline|anthropic|openai-compatible")
	model := fs.String("model", rulecompile.DefaultModel, "model id (Anthropic default claude-sonnet-4-5)")
	apiKeyEnv := fs.String("api-key-env", "ANTHROPIC_API_KEY", "env var holding API key for live providers")
	endpoint := fs.String("endpoint", "", "override API endpoint (Anthropic messages or OpenAI chat completions)")
	maxTokens := fs.Int("max-tokens", 4096, "max tokens per compile completion")
	opengrep := fs.String("opengrep-bin", "", "path to OpenGrep binary (required for fixture gates unless --skip-gate)")
	limit := fs.Int("limit", 0, "max obligations to compile (0 = all matching)")
	modalities := fs.String("modalities", "must,must_not", "comma-separated modalities to include")
	minP := fs.Float64("min-confidence-p", 0, "minimum extract confidence_p to compile")
	adrID := fs.String("adr", "", "optional single ADR id filter")
	skipGate := fs.Bool("skip-gate", false, "skip OpenGrep fixture execution (structural checks only)")
	repair := fs.Bool("repair", true, "retry compile with gate errors (up to 3)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `adrcompile — ADR corpus → draft OpenGrep rules (compile-time LLM only)

Workflow:
  1. Ingest ADR markdown under --root
  2. Deterministically extract RFC 2119 candidates (Pathfinder extract)
  3. Ask Claude (or offline fixture) for a draft rule + bad/good fixtures
  4. Gate with OpenGrep: bad must match, good must not
  5. Write report.json / report.md + drafts/ (never auto-approves)

Examples:
  # Dry run (no API): offline drafts + OpenGrep gates
  adrcompile --root testdata/corpus/adrs --out /tmp/ac --provider offline \
    --opengrep-bin /path/to/opengrep --limit 3

  # Live Anthropic
  export ANTHROPIC_API_KEY=...
  adrcompile --root testdata/corpus/adrs --out /tmp/ac --provider anthropic \
    --model claude-sonnet-4-5 --opengrep-bin /path/to/opengrep --limit 20

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" {
		fs.Usage()
		return fmt.Errorf("--root is required")
	}

	provider, err := buildProvider(*providerName, *model, *apiKeyEnv, *endpoint, *maxTokens)
	if err != nil {
		return err
	}

	mods := splitCSV(*modalities)
	report, err := rulecompile.Compile(rulecompile.Options{
		Root:           *root,
		OutDir:         *out,
		Provider:       provider,
		OpenGrepBinary: *opengrep,
		Limit:          *limit,
		Modalities:     mods,
		MinConfidenceP: *minP,
		ADRFilter:      strings.TrimSpace(*adrID),
		SkipGate:       *skipGate,
		Repair:         *repair,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Compiled %d / %d candidates → %s\n", report.CompiledTotal, report.CandidateTotal, report.OutDir)
	fmt.Printf("Gates: passed=%d failed=%d | bind_existing=%d inexpressible=%d manual_review=%d\n",
		report.GatePassed, report.GateFailed, report.BindExisting, report.Inexpressible, report.ManualReview)
	fmt.Printf("Tokens: prompt=%d completion=%d\n", report.PromptTokens, report.CompletionTokens)
	fmt.Printf("Reports: %s/report.json  %s/report.md\n", *out, *out)
	fmt.Println("Reminder: drafts are not Pathfinder policy until human review binds them.")
	return nil
}

func buildProvider(name, model, apiKeyEnv, endpoint string, maxTokens int) (rulecompile.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "offline":
		mid := model
		if mid == "" || mid == rulecompile.DefaultModel {
			mid = "offline-fixture"
		}
		return rulecompile.OfflineProvider{ModelID: mid}, nil
	case "anthropic":
		key := strings.TrimSpace(os.Getenv(apiKeyEnv))
		if key == "" && apiKeyEnv != "ANTHROPIC_API_KEY" {
			// also try default if custom env empty
			key = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
		return rulecompile.AnthropicProvider{
			APIKey:    key,
			ModelID:   model,
			MaxTokens: maxTokens,
			Endpoint:  endpoint,
		}, nil
	case "openai-compatible":
		key := strings.TrimSpace(os.Getenv(apiKeyEnv))
		if key == "" {
			key = strings.TrimSpace(os.Getenv("LLM_API_KEY"))
		}
		if model == rulecompile.DefaultModel {
			return nil, fmt.Errorf("--model is required for openai-compatible")
		}
		return rulecompile.OpenAICompatProvider{
			Endpoint:  endpoint,
			APIKey:    key,
			ModelID:   model,
			MaxTokens: maxTokens,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q (want offline|anthropic|openai-compatible)", name)
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
