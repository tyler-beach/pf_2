package obligations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/tyler-beach/pf_2/internal/adringest"
)

const (
	maxADRSourceBytes = 4 << 20
	maxCandidateLineBytes = 16 << 10
)

// Candidate is a source-cited interpretation suggestion, never an approved policy.
type Candidate struct {
	ID              string `json:"id"`
	ADRID           string `json:"adr_id"`
	ADRTitle        string `json:"adr_title"`
	ADRStatus       string `json:"adr_status,omitempty"`
	SectionHeading  string `json:"section_heading"`
	Statement       string `json:"statement"`
	Modality        string `json:"modality"`
	Kind            string `json:"kind"`
	PatternHint     string `json:"pattern_hint"`
	SourcePath      string `json:"source_path"`
	SourceSHA256    string `json:"source_sha256"`
	SourceLineStart int    `json:"source_line_start"`
	SourceLineEnd   int    `json:"source_line_end"`
	ApprovalStatus  string `json:"approval_status"`
	EnforcementMode string `json:"enforcement_mode"`
	Executable      bool   `json:"executable"`
	ReviewHints     []string `json:"review_hints,omitempty"`
}

// Index is a deterministic candidate inventory tied to exact input bytes.
type Index struct {
	SchemaVersion          string      `json:"schema_version"`
	SourceRoot              string      `json:"source_root"`
	SourceIndexSHA256       string      `json:"source_index_sha256"`
	DocumentCount           int         `json:"document_count"`
	DocumentsWithCandidates int         `json:"documents_with_candidates"`
	CandidateCount          int         `json:"candidate_count"`
	Candidates              []Candidate `json:"candidates"`
}

// BuildIndex verifies indexed ADR metadata against current source bytes, then
// generates unapproved candidates. It never writes the active requirements catalog.
func BuildIndex(documents []adringest.Document, sourceRootPath, sourceRootLabel string, sourceIndexRaw []byte) (Index, error) {
	var zero Index
	if len(sourceIndexRaw) == 0 {
		return zero, fmt.Errorf("source ingestion index is empty")
	}
	if strings.TrimSpace(sourceRootPath) == "" {
		return zero, fmt.Errorf("source root path is required")
	}
	rootAbs, err := filepath.Abs(sourceRootPath)
	if err != nil { return zero, fmt.Errorf("resolve ADR source root: %w", err) }
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil { return zero, fmt.Errorf("resolve ADR source root symlinks: %w", err) }
	info, err := os.Stat(rootReal)
	if err != nil { return zero, fmt.Errorf("inspect ADR source root: %w", err) }
	if !info.IsDir() { return zero, fmt.Errorf("ADR source root %q is not a directory", rootReal) }
	label, err := normalizeSourceRootLabel(sourceRootLabel)
	if err != nil { return zero, err }

	docs := append([]adringest.Document(nil), documents...)
	sort.Slice(docs, func(i, j int) bool { return filepath.ToSlash(docs[i].SourcePath) < filepath.ToSlash(docs[j].SourcePath) })
	sum := sha256.Sum256(sourceIndexRaw)
	out := Index{
		SchemaVersion: "1",
		SourceRoot: label,
		SourceIndexSHA256: hex.EncodeToString(sum[:]),
		DocumentCount: len(docs),
		Candidates: []Candidate{},
	}
	seen := make(map[string]string, len(docs))
	for _, doc := range docs {
		if strings.TrimSpace(doc.ID) == "" { return zero, fmt.Errorf("ingestion index contains an empty ADR ID") }
		if previous, exists := seen[doc.ID]; exists {
			return zero, fmt.Errorf("duplicate ADR ID %q in source paths %q and %q", doc.ID, previous, doc.SourcePath)
		}
		seen[doc.ID] = doc.SourcePath
		if !validSHA256(doc.SourceSHA256) { return zero, fmt.Errorf("ADR %q has invalid source SHA-256 metadata", doc.ID) }
		source, err := readContainedSource(rootReal, doc.SourcePath)
		if err != nil { return zero, fmt.Errorf("load source for %s: %w", doc.ID, err) }
		candidates, err := ExtractDocument(doc, joinSourcePath(label, doc.SourcePath), source)
		if err != nil { return zero, fmt.Errorf("extract candidates from %s: %w", doc.SourcePath, err) }
		if len(candidates) > 0 { out.DocumentsWithCandidates++ }
		out.Candidates = append(out.Candidates, candidates...)
	}
	sort.Slice(out.Candidates, func(i, j int) bool {
		if out.Candidates[i].SourcePath != out.Candidates[j].SourcePath { return out.Candidates[i].SourcePath < out.Candidates[j].SourcePath }
		if out.Candidates[i].SourceLineStart != out.Candidates[j].SourceLineStart { return out.Candidates[i].SourceLineStart < out.Candidates[j].SourceLineStart }
		return out.Candidates[i].ID < out.Candidates[j].ID
	})
	out.CandidateCount = len(out.Candidates)
	return out, nil
}

// MarkdownCandidateReport renders a reviewer-facing queue. It never marks a
// candidate approved or executable.
func MarkdownCandidateReport(index Index) string {
	var b strings.Builder
	b.WriteString("# Obligation candidate review queue\n\n")
	b.WriteString("> Review queue only: extracted candidates are not approvals and are not executable.\n\n")
	fmt.Fprintf(&b, "- Source root: `%s`\n", mdCell(index.SourceRoot))
	fmt.Fprintf(&b, "- Source ingestion index SHA-256: `%s`\n", index.SourceIndexSHA256)
	fmt.Fprintf(&b, "- Documents: %d; documents with candidates: %d; candidates: %d\n\n",
		index.DocumentCount, index.DocumentsWithCandidates, index.CandidateCount)
	if len(index.Candidates) == 0 {
		b.WriteString("No normative candidate statements were found. This is not evidence that the ADR corpus has no obligations.\n")
		return b.String()
	}
	b.WriteString("| Candidate | Source | Section | Modality | Pattern hint | Review hints | Statement | State |\n|---|---|---|---|---|---|---|---|\n")
	for _, candidate := range index.Candidates {
		fmt.Fprintf(&b, "| %s | %s:%d-%d | %s | %s | %s | %s | %s | %s / %s |\n",
			mdCell(candidate.ID), mdCell(candidate.SourcePath), candidate.SourceLineStart, candidate.SourceLineEnd,
			mdCell(candidate.SectionHeading), mdCell(candidate.Modality), mdCell(candidate.PatternHint),
			mdCell(strings.Join(candidate.ReviewHints, ", ")), mdCell(candidate.Statement),
			mdCell(candidate.ApprovalStatus), mdCell(candidate.EnforcementMode))
	}
	return b.String()
}

// ExtractDocument accepts only source bytes matching indexed ID, title and SHA.
// It ignores fenced examples and records an exact original source line for each
// candidate; natural-language suggestions are deliberately not auto-enforced.
func ExtractDocument(doc adringest.Document, displayPath string, source []byte) ([]Candidate, error) {
	if len(source) > maxADRSourceBytes { return nil, fmt.Errorf("ADR source exceeds the %d-byte limit", maxADRSourceBytes) }
	parsed, err := adringest.Parse(doc.SourcePath, source)
	if err != nil { return nil, fmt.Errorf("reparse source ADR: %w", err) }
	if parsed.ID != doc.ID || parsed.Title != doc.Title || parsed.SourceSHA256 != doc.SourceSHA256 {
		return nil, fmt.Errorf("ADR source no longer matches the indexed ID, title, or SHA-256")
	}
	cleanDisplayPath, err := cleanRelativePath(displayPath)
	if err != nil { return nil, fmt.Errorf("candidate display path: %w", err) }
	lines := strings.Split(string(source), "\n")
	result := make([]Candidate, 0)
	for _, section := range parsed.Sections {
		start, end := section.StartLine, section.EndLine
		if start < 1 || end < start || end > len(lines) {
			return nil, fmt.Errorf("section %q has invalid source line range %d-%d", section.Heading, start, end)
		}
		inFence := false
		for lineIndex := start; lineIndex < end; lineIndex++ {
			trimmed := strings.TrimSpace(lines[lineIndex])
			if isFenceLine(trimmed) { inFence = !inFence; continue }
			if inFence || trimmed == "" || strings.HasPrefix(trimmed, "<!--") { continue }
			statement := normalizeStatement(trimmed)
			if statement == "" { continue }
			modality, ok := detectModality(statement)
			if !ok { continue }
			if len(statement) > maxCandidateLineBytes {
				return nil, fmt.Errorf("normative source line %d exceeds the %d-byte candidate statement limit", lineIndex+1, maxCandidateLineBytes)
			}
			kind := "normative_candidate"
			if anyPhrase(statement, "unless", "except", "only when", "other than", "waiver", "exception") { kind = "exception_candidate" }
			candidate := Candidate{
				ADRID: parsed.ID, ADRTitle: parsed.Title, ADRStatus: parsed.Status,
				SectionHeading: section.Heading, Statement: statement, Modality: modality,
				Kind: kind, PatternHint: suggestPattern(statement), SourcePath: cleanDisplayPath,
				SourceSHA256: parsed.SourceSHA256, SourceLineStart: lineIndex+1, SourceLineEnd: lineIndex+1,
				ApprovalStatus: "pending_review", EnforcementMode: "review_required", Executable: false,
				ReviewHints: reviewHints(statement, kind, section.Heading),
			}
			candidate.ID = candidateID(candidate)
			result = append(result, candidate)
		}
	}
	return result, nil
}

func detectModality(statement string) (string, bool) {
	// Use the first normative cue in the sentence, not whichever modality has
	// the highest global priority. For example, "An exception may be approved
	// only when ... requires it" is permission conditioned on a requirement,
	// not an unconditional MUST. At the same position, prefer the longer phrase
	// so "must not" is not reduced to "must".
	type cue struct { phrase, modality string }
	cues := []cue{
		{"not required", "not_required"},
		{"must not", "must_not"}, {"mustn't", "must_not"},
		{"shall not", "must_not"}, {"should not", "must_not"},
		{"may not", "must_not"}, {"do not", "must_not"},
		{"does not", "must_not"}, {"never", "must_not"},
		{"prohibited", "must_not"}, {"forbidden", "must_not"},
		{"cannot", "must_not"}, {"can't", "must_not"},
		{"not permitted", "must_not"},
		{"must", "must"}, {"shall", "must"}, {"required", "must"},
		{"requires", "must"}, {"has to", "must"}, {"have to", "must"},
		{"should", "should"},
		{"may", "may"}, {"permitted", "may"}, {"allowed", "may"},
	}
	bestIndex, bestLength, bestModality := -1, -1, ""
	for _, candidate := range cues {
		index := firstPhraseIndex(statement, candidate.phrase)
		if index < 0 { continue }
		if bestIndex < 0 || index < bestIndex || (index == bestIndex && len(candidate.phrase) > bestLength) {
			bestIndex, bestLength, bestModality = index, len(candidate.phrase), candidate.modality
		}
	}
	if bestIndex < 0 { return "", false }
	return bestModality, true
}

// firstPhraseIndex returns the first case-insensitive whole-word occurrence.
func firstPhraseIndex(value, phrase string) int {
	lowerValue, lowerPhrase := strings.ToLower(value), strings.ToLower(phrase)
	offset := 0
	for offset <= len(lowerValue)-len(lowerPhrase) {
		idx := strings.Index(lowerValue[offset:], lowerPhrase)
		if idx < 0 { return -1 }
		start := offset + idx
		end := start + len(lowerPhrase)
		leftOK := start == 0 || !isWordRune(runeAtEnd(lowerValue[:start]))
		rightOK := end == len(lowerValue) || !isWordRune(runeAtStart(lowerValue[end:]))
		if leftOK && rightOK { return start }
		offset = start + 1
	}
	return -1
}

// anyPhrase performs case-insensitive phrase matching on word boundaries so
// words like "mustache" are not treated as normative requirements.
func anyPhrase(text string, phrases ...string) bool {
	for _, phrase := range phrases {
		lowerText, lowerPhrase := strings.ToLower(text), strings.ToLower(phrase)
		offset := 0
		for offset <= len(lowerText)-len(lowerPhrase) {
			idx := strings.Index(lowerText[offset:], lowerPhrase)
			if idx < 0 { break }
			start := offset + idx
			end := start + len(lowerPhrase)
			leftOK := start == 0 || !isWordRune(runeAtEnd(lowerText[:start]))
			rightOK := end == len(lowerText) || !isWordRune(runeAtStart(lowerText[end:]))
			if leftOK && rightOK { return true }
			offset = start + 1
		}
	}
	return false
}

func runeAtEnd(value string) rune {
	for _, r := range value { _ = r }
	runes := []rune(value)
	if len(runes) == 0 { return 0 }
	return runes[len(runes)-1]
}

func runeAtStart(value string) rune {
	for _, r := range value { return r }
	return 0
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// reviewHints marks deterministic extraction risks for a human reviewer. Hints
// never change the candidate's review-required state and never activate a verifier.
func reviewHints(statement, kind, sectionHeading string) []string {
	lower := strings.ToLower(statement)
	hints := make([]string, 0, 5)
	if isNonDecisionSection(sectionHeading) { hints = append(hints, "non_decision_section") }
	if kind == "exception_candidate" || anyPhrase(statement, "unless", "except", "only when", "provided that", "waiver", "exception") {
		hints = append(hints, "condition_or_exception")
	}
	if strings.Contains(lower, ";") || strings.Contains(lower, " and ") || strings.Contains(lower, " or ") ||
		(anyPhrase(statement, "finite", "bounded") && anyPhrase(statement, "no greater than", "at most", "maximum", "less than or equal to")) {
		hints = append(hints, "compound_or_conjoined")
	}
	if anyPhrase(statement, "if", "when", "only when", "provided that", "unless") {
		hints = append(hints, "conditional_scope")
	}
	if anyPhrase(statement, "appropriate", "reasonable", "timely", "as needed", "best effort", "sufficient", "adequate", "quickly", "secure") {
		hints = append(hints, "subjective_or_underspecified")
	}
	return hints
}

func isNonDecisionSection(heading string) bool {
	value := strings.ToLower(strings.TrimSpace(heading))
	value = strings.Trim(value, " .:-")
	for _, prefix := range []string{
		"context", "background", "motivation", "example", "examples",
		"alternatives considered", "rejected alternatives", "non-goal", "non-goals",
		"references", "appendix", "appendices",
	} {
		if value == prefix || strings.HasPrefix(value, prefix+":") || strings.HasPrefix(value, prefix+" ") {
			return true
		}
	}
	return false
}

func suggestPattern(statement string) string {
	lower := strings.ToLower(statement)
	switch {
	case anyPhrase(lower, "timeout", "retry", "retries", "attempt", "attempts", "backoff", "configuration", "config", "required key", "setting"):
		return "configuration.bounds_or_required_values"
	case anyPhrase(lower, "shell", "command", "process.start", "runtime.exec", "eval", "credential", "secret"):
		return "security.source_pattern"
	case anyPhrase(lower, "dependency", "import", "package", "coupling", "gateway", "service boundary"):
		return "architecture.dependency_or_boundary"
	default:
		return "manual_review"
	}
}

func normalizeStatement(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
	trimmed := strings.TrimLeft(line, "-*+ ")
	if trimmed != line { line = strings.TrimSpace(trimmed) } else {
		i := 0
		for i < len(line) && line[i] >= '0' && line[i] <= '9' { i++ }
		if i > 0 && i+1 < len(line) && (line[i] == '.' || line[i] == ')') && line[i+1] == ' ' { line = strings.TrimSpace(line[i+2:]) }
	}
	return strings.TrimSpace(line)
}

func isFenceLine(line string) bool {
	ticks := strings.Repeat(string(rune(96)), 3)
	return strings.HasPrefix(line, ticks) || strings.HasPrefix(line, "~~~")
}

func candidateID(c Candidate) string {
	value := strings.Join([]string{c.ADRID, c.SourcePath, c.SectionHeading, fmt.Sprintf("%d", c.SourceLineStart), c.Statement}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return "CAND-" + hex.EncodeToString(sum[:8])
}

func normalizeSourceRootLabel(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" { return ".", nil }
	root = filepath.ToSlash(root)
	if filepath.IsAbs(filepath.FromSlash(root)) || (len(root) >= 2 && root[1] == ':') {
		root = filepath.Base(filepath.Clean(filepath.FromSlash(root)))
	} else {
		root = filepath.ToSlash(filepath.Clean(filepath.FromSlash(root)))
	}
	if root == "" || root == ".." || strings.HasPrefix(root, "../") {
		return "", fmt.Errorf("source_root must be repository-relative or an absolute root with a stable directory name")
	}
	return root, nil
}

func joinSourcePath(rootLabel, sourcePath string) string {
	clean, _ := cleanRelativePath(sourcePath)
	if rootLabel == "" || rootLabel == "." { return clean }
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(rootLabel), filepath.FromSlash(clean)))
}

func readContainedSource(root, sourcePath string) ([]byte, error) {
	relative, err := cleanRelativePath(sourcePath)
	if err != nil { return nil, err }
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil { return nil, fmt.Errorf("resolve source file %q: %w", relative, err) }
	if filepath.Clean(candidate) != filepath.Clean(resolved) { return nil, fmt.Errorf("source file %q uses a symlinked path", relative) }
	rel, err := filepath.Rel(root, resolved)
	if err != nil { return nil, fmt.Errorf("validate source containment: %w", err) }
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("source path %q escapes the ADR root", relative)
	}
	info, err := os.Stat(resolved)
	if err != nil { return nil, fmt.Errorf("inspect source file %q: %w", relative, err) }
	if !info.Mode().IsRegular() { return nil, fmt.Errorf("source %q is not a regular file", relative) }
	if info.Size() > maxADRSourceBytes { return nil, fmt.Errorf("source %q exceeds the %d-byte limit", relative, maxADRSourceBytes) }
	data, err := os.ReadFile(resolved)
	if err != nil { return nil, fmt.Errorf("read source file %q: %w", relative, err) }
	return data, nil
}

func cleanRelativePath(sourcePath string) (string, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(sourcePath)))
	unsafe := sourcePath == "" || strings.ContainsRune(sourcePath, 0) || filepath.IsAbs(filepath.FromSlash(clean)) ||
		clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || (len(clean) >= 2 && clean[1] == ':')
	if unsafe { return "", fmt.Errorf("source path %q must be a safe repository-relative path", sourcePath) }
	return clean, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 { return false }
	_, err := hex.DecodeString(value)
	return err == nil
}
