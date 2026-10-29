package rulecompile

import (
	"fmt"
	"strings"

	"github.com/tyler-beach/pf_2/internal/obligations"
)

const systemPrompt = `You are Pathfinder's ADR rule compiler. You turn ONE RFC 2119 obligation into a draft OpenGrep/Semgrep-compatible YAML rule plus fixtures.

Rules:
- Reply with a single JSON object only (no markdown).
- Never mark anything approved or executable.
- Prefer bind_existing when a known rule id clearly covers the obligation.
- Use compile_new only when a structural OpenGrep pattern can express the obligation.
- Use inexpressible for process/org/semantic obligations OpenGrep cannot prove.
- Use manual_review when ambiguous.
- Fixtures must be minimal and language-correct.
- rule_yaml must be valid Semgrep/OpenGrep YAML with a unique rules[0].id under pathfinder.compiled.*.
- llm_confidence is your calibrated certainty the draft matches the statement (0-1).

JSON schema:
{
  "route": "compile_new"|"bind_existing"|"inexpressible"|"manual_review",
  "llm_confidence": 0.0,
  "rationale": "string",
  "pattern_id": "string",
  "existing_rule_ids": ["optional"],
  "languages": ["python"|"go"|"javascript"|...],
  "rule_id": "pathfinder.compiled....",
  "rule_yaml": "rules:\n  - id: ...\n",
  "bad_filename": "bad.py",
  "bad_content": "...",
  "good_filename": "good.py",
  "good_content": "..."
}`

func buildUserPrompt(c obligations.Candidate, gateFeedback string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Prompt-version: %s\n", PromptVersion)
	fmt.Fprintf(&b, "ADR: %s (%s)\n", c.ADRID, c.ADRTitle)
	fmt.Fprintf(&b, "Modality: %s\n", c.Modality)
	fmt.Fprintf(&b, "Pattern hint: %s\n", c.PatternHint)
	fmt.Fprintf(&b, "Extract confidence_p: %.3f (%s)\n", c.ConfidenceP, c.Confidence)
	fmt.Fprintf(&b, "Source: %s:%d-%d\n", c.SourcePath, c.SourceLineStart, c.SourceLineEnd)
	fmt.Fprintf(&b, "Statement:\n%s\n", c.Statement)
	if len(c.ReviewHints) > 0 {
		fmt.Fprintf(&b, "Review hints: %s\n", strings.Join(c.ReviewHints, "; "))
	}
	if len(c.Ambiguity) > 0 {
		fmt.Fprintf(&b, "Ambiguity: %s\n", strings.Join(c.Ambiguity, "; "))
	}
	if strings.TrimSpace(gateFeedback) != "" {
		fmt.Fprintf(&b, "\nPrevious draft failed OpenGrep gates. Repair the JSON draft:\n%s\n", gateFeedback)
	}
	b.WriteString("\nReturn the JSON draft now.")
	return b.String()
}
