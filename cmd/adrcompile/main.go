// Command adrcompile is a self-contained ADR→OpenGrep ruleset publisher.
//
// It extracts RFC 2119 obligations from a MADR corpus, compiles draft rules
// with optional Anthropic assistance, gates them with OpenGrep fixtures, and
// publishes a versioned ruleset package Pathfinder can pin via ruleset_sha256.
//
// Compile-time LLM only. Pathfinder assess/PR remains zero-LLM. Published
// bindings are suggestions — not approved requirements.json entries.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tyler-beach/pf_2/internal/rulecompile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "compile":
		err = runCompile(os.Args[2:])
	case "publish":
		err = runPublish(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		// Backward compatible: flags without subcommand ⇒ compile
		if strings.HasPrefix(os.Args[1], "-") {
			err = runCompile(os.Args[1:])
		} else {
			fmt.Fprintf(os.Stderr, "adrcompile: unknown command %q\n\n", os.Args[1])
			usage()
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "adrcompile: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `adrcompile — self-contained ADR corpus → versioned OpenGrep rulesets

Commands:
  compile   Extract obligations, LLM-draft rules, OpenGrep fixture gates
  publish   Package gate-passed rules into a versioned ruleset Pathfinder can pin
  verify    Check manifest digests for a published package

Typical flow:
  adrcompile compile --root testdata/corpus/adrs --out /tmp/ac-work \
      --provider anthropic --opengrep-bin /path/to/opengrep --limit 50

  adrcompile publish --from /tmp/ac-work --out /tmp/rulesets \
      --name pathfinder-adr-corpus --version 0.1.0 --tar

  adrcompile verify --package /tmp/rulesets/pathfinder-adr-corpus-0.1.0

Pathfinder consume:
  adrctl scan-opengrep --config <pkg>/rules.yml --binary <opengrep> ...
  # pin manifest.ruleset_sha256 in the analyzer plan

`)
}

func runCompile(args []string) error {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	root := fs.String("root", "", "ADR markdown directory")
	out := fs.String("out", "adrcompile-out", "workdir for candidates/drafts/report")
	providerName := fs.String("provider", "offline", "offline|anthropic|openai-compatible")
	model := fs.String("model", rulecompile.DefaultModel, "model id")
	apiKeyEnv := fs.String("api-key-env", "ANTHROPIC_API_KEY", "env var for API key")
	endpoint := fs.String("endpoint", "", "API endpoint override")
	maxTokens := fs.Int("max-tokens", 4096, "max tokens per completion")
	opengrep := fs.String("opengrep-bin", "", "OpenGrep binary (required unless --skip-gate)")
	limit := fs.Int("limit", 0, "max obligations (0=all)")
	modalities := fs.String("modalities", "must,must_not", "modalities to include")
	minP := fs.Float64("min-confidence-p", 0, "min extract confidence_p")
	adrID := fs.String("adr", "", "optional ADR id filter")
	skipGate := fs.Bool("skip-gate", false, "structural checks only")
	repair := fs.Bool("repair", true, "retry on gate failure")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" {
		return fmt.Errorf("compile: --root is required")
	}
	provider, err := buildProvider(*providerName, *model, *apiKeyEnv, *endpoint, *maxTokens)
	if err != nil {
		return err
	}
	report, err := rulecompile.Compile(rulecompile.Options{
		Root:           *root,
		OutDir:         *out,
		Provider:       provider,
		OpenGrepBinary: *opengrep,
		Limit:          *limit,
		Modalities:     splitCSV(*modalities),
		MinConfidenceP: *minP,
		ADRFilter:      strings.TrimSpace(*adrID),
		SkipGate:       *skipGate,
		Repair:         *repair,
	})
	if err != nil {
		return err
	}
	fmt.Printf("compile: %d/%d candidates → %s\n", report.CompiledTotal, report.CandidateTotal, report.OutDir)
	fmt.Printf("gates: passed=%d failed=%d | bind_existing=%d inexpressible=%d manual_review=%d\n",
		report.GatePassed, report.GateFailed, report.BindExisting, report.Inexpressible, report.ManualReview)
	fmt.Printf("next: adrcompile publish --from %s --out ./rulesets --name pathfinder-adr-corpus --version 0.1.0 --tar\n", *out)
	return nil
}

func runPublish(args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	from := fs.String("from", "", "compile workdir containing report.json")
	out := fs.String("out", "rulesets", "directory to write {name}-{version}/")
	name := fs.String("name", "pathfinder-adr-rules", "ruleset name")
	version := fs.String("version", "", "semver (required), e.g. 0.1.0")
	tar := fs.Bool("tar", false, "also write .tar.gz")
	includeDrafts := fs.Bool("include-drafts", false, "copy draft provenance into package (large)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*version) == "" {
		return fmt.Errorf("publish: --from and --version are required")
	}
	res, err := rulecompile.Publish(rulecompile.PublishOptions{
		Name:          *name,
		Version:       *version,
		FromDir:       *from,
		OutDir:        *out,
		TarGz:         *tar,
		IncludeDrafts: *includeDrafts,
	})
	if err != nil {
		return err
	}
	fmt.Printf("publish: %s\n", res.PackageDir)
	fmt.Printf("rules: %d  ruleset_sha256=%s\n", res.Manifest.RuleCount, res.Manifest.RulesetSHA256)
	fmt.Printf("manifest: %s\n", res.ManifestPath)
	if res.TarGzPath != "" {
		fmt.Printf("archive: %s\n", res.TarGzPath)
	}
	fmt.Printf("pathfinder: scan-opengrep --config %s/rules.yml  (pin ruleset_sha256)\n", res.PackageDir)
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pkg := fs.String("package", "", "published package directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*pkg) == "" {
		return fmt.Errorf("verify: --package is required")
	}
	res, err := rulecompile.VerifyPackage(*pkg)
	if err != nil {
		return err
	}
	if !res.OK {
		for _, e := range res.Errors {
			fmt.Fprintf(os.Stderr, "verify: %s\n", e)
		}
		return fmt.Errorf("package verification failed (%d errors)", len(res.Errors))
	}
	fmt.Printf("verify: ok rules=%d ruleset_sha256=%s\n", res.RuleCount, res.RulesetSHA256)
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
			key = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
		return rulecompile.AnthropicProvider{
			APIKey: key, ModelID: model, MaxTokens: maxTokens, Endpoint: endpoint,
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
			Endpoint: endpoint, APIKey: key, ModelID: model, MaxTokens: maxTokens,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", name)
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
