package rulecompile

import (
	"fmt"
	"strings"
)

// RenderMarkdown produces a human-readable compile summary.
func RenderMarkdown(r Report) string {
	var b strings.Builder
	b.WriteString("# ADR → OpenGrep compile report\n\n")
	fmt.Fprintf(&b, "- Provider: `%s` / `%s`\n", r.Provider, r.Model)
	fmt.Fprintf(&b, "- Source root: `%s`\n", r.SourceRoot)
	fmt.Fprintf(&b, "- Candidates extracted: %d; selected for compile: %d\n", r.CandidateTotal, r.CompiledTotal)
	fmt.Fprintf(&b, "- Gates passed: %d; failed: %d\n", r.GatePassed, r.GateFailed)
	fmt.Fprintf(&b, "- bind_existing: %d; inexpressible: %d; manual_review: %d\n", r.BindExisting, r.Inexpressible, r.ManualReview)
	fmt.Fprintf(&b, "- Tokens: prompt=%d completion=%d\n\n", r.PromptTokens, r.CompletionTokens)
	b.WriteString("> Drafts are **not** approved policy. Bind via Pathfinder review before CI enforce.\n\n")
	b.WriteString("| ADR | Modality | Route | Gate | Combined p | Statement |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, it := range r.Items {
		stmt := it.Statement
		if len(stmt) > 80 {
			stmt = stmt[:77] + "..."
		}
		stmt = strings.ReplaceAll(stmt, "|", "\\|")
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %.2f (%s) | %s |\n",
			it.ADRID, it.Modality, it.Draft.Route, it.Gate.Status, it.CombinedConfidenceP, it.CombinedConfidence, stmt)
	}
	b.WriteString("\n## Notes\n\n")
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	return b.String()
}
