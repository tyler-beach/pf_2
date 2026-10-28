package assessment

import (
	"fmt"
	"strings"
)

// SubmissionKind classifies the primary input shape entering assess / extract.
// Parsers and trends differ for English tech-spec prose versus source/IaC snippets.
// Empty means unspecified (back-compat for older artifacts).
type SubmissionKind string

const (
	SubmissionKindUnspecified SubmissionKind = ""
	SubmissionKindTechSpec    SubmissionKind = "tech_spec"
	SubmissionKindCodeSnippet SubmissionKind = "code_snippet"
)

// ParseSubmissionKind validates an explicit CLI/API value.
// Empty string is allowed and means unspecified.
func ParseSubmissionKind(raw string) (SubmissionKind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "unspecified", "unknown":
		return SubmissionKindUnspecified, nil
	case "tech_spec", "tech-spec", "techspec", "adr", "prose":
		return SubmissionKindTechSpec, nil
	case "code_snippet", "code-snippet", "codesnippet", "code", "iac", "snippet":
		return SubmissionKindCodeSnippet, nil
	default:
		return "", fmt.Errorf("invalid submission kind %q (want tech_spec, code_snippet, or empty)", raw)
	}
}

// ClassifyIssueSubmissionKind infers kind from an issue Markdown body.
// Fence-only → code_snippet; substantial non-fence prose → tech_spec;
// both or neither → unspecified (callers may override with an explicit flag).
func ClassifyIssueSubmissionKind(markdown string) SubmissionKind {
	files := ParseIssueMarkdown(markdown)
	hasSnippets := len(files) > 0
	hasProse := issueHasTechSpecProse(markdown)
	switch {
	case hasSnippets && !hasProse:
		return SubmissionKindCodeSnippet
	case hasProse && !hasSnippets:
		return SubmissionKindTechSpec
	default:
		return SubmissionKindUnspecified
	}
}

// DefaultRepositorySubmissionKind is the natural kind for assess-repo trees
// (source / IaC scanned by OpenGrep / Checkov). Issue bodies often carry tech_spec.
func DefaultRepositorySubmissionKind() SubmissionKind {
	return SubmissionKindCodeSnippet
}

// ResolveSubmissionKind prefers an explicit kind; otherwise uses the hint.
func ResolveSubmissionKind(explicit, hint SubmissionKind) SubmissionKind {
	if explicit != SubmissionKindUnspecified {
		return explicit
	}
	return hint
}

func issueHasTechSpecProse(markdown string) bool {
	stripped := stripFencedBlocks(markdown)
	var kept []string
	for _, line := range strings.Split(stripped, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "### File:") {
			continue
		}
		kept = append(kept, trim)
	}
	if len(kept) == 0 {
		return false
	}
	joined := strings.Join(kept, "\n")
	lower := strings.ToLower(joined)
	if strings.Contains(lower, "## decision") || strings.Contains(lower, "# adr") ||
		strings.Contains(lower, "architecture decision") || strings.Contains(lower, "tech spec") ||
		strings.Contains(lower, "technical specification") {
		return true
	}
	// Any multi-sentence prose outside fences counts as tech-spec-ish English.
	letters := 0
	for _, r := range joined {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters++
		}
	}
	return letters >= 80
}

func stripFencedBlocks(markdown string) string {
	fence := "```"
	lines := strings.Split(markdown, "\n")
	var out []string
	inFence := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, fence) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
