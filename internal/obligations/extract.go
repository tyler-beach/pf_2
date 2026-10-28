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
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations/calibration"
)

const (
	maxADRSourceBytes     = 4 << 20
	maxCandidateLineBytes = 16 << 10
)

var sentenceAbbreviations = []string{"e.g.", "i.e.", "vs.", "mr.", "mrs.", "ms.", "dr.", "prof.", "fig.", "approx.", "u.s.", "u.k.", "e.u.", "u.n."}

// Candidate is a source-cited interpretation suggestion, never an approved policy.
type Candidate struct {
	ID              string   `json:"id"`
	ADRID           string   `json:"adr_id"`
	ADRTitle        string   `json:"adr_title"`
	ADRStatus       string   `json:"adr_status,omitempty"`
	SectionHeading  string   `json:"section_heading"`
	Statement       string   `json:"statement"`
	Modality        string   `json:"modality"`
	Kind            string   `json:"kind"`
	// SubmissionKind is tech_spec for ADR/prose extraction (English parsers).
	// Empty means unspecified for older candidate indexes.
	SubmissionKind  string   `json:"submission_kind,omitempty"`
	PatternHint     string   `json:"pattern_hint"`
	SourcePath      string   `json:"source_path"`
	SourceSHA256    string   `json:"source_sha256"`
	SourceLineStart int      `json:"source_line_start"`
	SourceLineEnd   int      `json:"source_line_end"`
	ApprovalStatus  string   `json:"approval_status"`
	EnforcementMode string   `json:"enforcement_mode"`
	Executable      bool     `json:"executable"`
	ReviewHints     []string `json:"review_hints,omitempty"`
	// Structured extraction aids for reviewers; never auto-enforce.
	Conditions []string `json:"conditions,omitempty"`
	Exceptions []string `json:"exceptions,omitempty"`
	Ambiguity  []string `json:"ambiguity,omitempty"`
	// Deterministic statistical confidence; never produced by an LLM.
	// Confidence is the conformal triage band; ConfidenceP is calibrated P(extract_correct).
	Confidence         string   `json:"confidence"`
	ConfidenceP        float64  `json:"confidence_p"`
	ConfidenceMethod   string   `json:"confidence_method,omitempty"`
	ConfidenceCoverage float64  `json:"confidence_coverage,omitempty"`
	PredictionSet      []string `json:"prediction_set,omitempty"`
	EscalationReasons  []string `json:"escalation_reasons,omitempty"`
}

// CalibrationMeta summarizes which statistical confidence method produced bands.
type CalibrationMeta struct {
	Method             string  `json:"method"`
	Coverage           float64 `json:"coverage,omitempty"`
	GoldExampleCount   int     `json:"gold_example_count,omitempty"`
	ParamsVersion      string  `json:"params_version,omitempty"`
	ExchangeabilityNote string `json:"exchangeability_note,omitempty"`
}

// Index is a deterministic candidate inventory tied to exact input bytes.
type Index struct {
	SchemaVersion           string            `json:"schema_version"`
	SourceRoot              string            `json:"source_root"`
	SourceIndexSHA256       string            `json:"source_index_sha256"`
	DocumentCount           int               `json:"document_count"`
	DocumentsWithCandidates int               `json:"documents_with_candidates"`
	CandidateCount          int               `json:"candidate_count"`
	ConfidenceSummary       ConfidenceSummary `json:"confidence_summary"`
	Calibration             CalibrationMeta   `json:"calibration,omitempty"`
	Candidates              []Candidate       `json:"candidates"`
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
	if err != nil {
		return zero, fmt.Errorf("resolve ADR source root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return zero, fmt.Errorf("resolve ADR source root symlinks: %w", err)
	}
	info, err := os.Stat(rootReal)
	if err != nil {
		return zero, fmt.Errorf("inspect ADR source root: %w", err)
	}
	if !info.IsDir() {
		return zero, fmt.Errorf("ADR source root %q is not a directory", rootReal)
	}
	label, err := normalizeSourceRootLabel(sourceRootLabel)
	if err != nil {
		return zero, err
	}

	docs := append([]adringest.Document(nil), documents...)
	sort.Slice(docs, func(i, j int) bool {
		return filepath.ToSlash(docs[i].SourcePath) < filepath.ToSlash(docs[j].SourcePath)
	})
	sum := sha256.Sum256(sourceIndexRaw)
	out := Index{
		SchemaVersion:     "1",
		SourceRoot:        label,
		SourceIndexSHA256: hex.EncodeToString(sum[:]),
		DocumentCount:     len(docs),
		Candidates:        []Candidate{},
	}
	seen := make(map[string]string, len(docs))
	for _, doc := range docs {
		if strings.TrimSpace(doc.ID) == "" {
			return zero, fmt.Errorf("ingestion index contains an empty ADR ID")
		}
		if previous, exists := seen[doc.ID]; exists {
			return zero, fmt.Errorf("duplicate ADR ID %q in source paths %q and %q", doc.ID, previous, doc.SourcePath)
		}
		seen[doc.ID] = doc.SourcePath
		if !validSHA256(doc.SourceSHA256) {
			return zero, fmt.Errorf("ADR %q has invalid source SHA-256 metadata", doc.ID)
		}
		source, err := readContainedSource(rootReal, doc.SourcePath)
		if err != nil {
			return zero, fmt.Errorf("load source for %s: %w", doc.ID, err)
		}
		candidates, err := ExtractDocument(doc, joinSourcePath(label, doc.SourcePath), source)
		if err != nil {
			return zero, fmt.Errorf("extract candidates from %s: %w", doc.SourcePath, err)
		}
		if len(candidates) > 0 {
			out.DocumentsWithCandidates++
		}
		out.Candidates = append(out.Candidates, candidates...)
	}
	sort.Slice(out.Candidates, func(i, j int) bool {
		if out.Candidates[i].SourcePath != out.Candidates[j].SourcePath {
			return out.Candidates[i].SourcePath < out.Candidates[j].SourcePath
		}
		if out.Candidates[i].SourceLineStart != out.Candidates[j].SourceLineStart {
			return out.Candidates[i].SourceLineStart < out.Candidates[j].SourceLineStart
		}
		return out.Candidates[i].ID < out.Candidates[j].ID
	})
	out.CandidateCount = len(out.Candidates)
	out.ConfidenceSummary = SummarizeConfidence(out)
	params := calibration.DefaultParams()
	out.Calibration = CalibrationMeta{
		Method:              params.Method,
		Coverage:            params.ConformalTargetCoverage,
		GoldExampleCount:    params.GoldExampleCount,
		ParamsVersion:       params.Version,
		ExchangeabilityNote: params.ExchangeabilityNote,
	}
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
	summary := index.ConfidenceSummary
	if summary.High+summary.Medium+summary.Low == 0 {
		summary = SummarizeConfidence(index)
	}
	fmt.Fprintf(&b, "- Documents: %d; documents with candidates: %d; candidates: %d\n",
		index.DocumentCount, index.DocumentsWithCandidates, index.CandidateCount)
	fmt.Fprintf(&b, "- Confidence: high=%d medium=%d low=%d (low conformal/heuristic band is assist-eligible)\n",
		summary.High, summary.Medium, summary.Low)
	if index.Calibration.Method != "" {
		fmt.Fprintf(&b, "- Calibration method: `%s`", index.Calibration.Method)
		if index.Calibration.Coverage > 0 {
			fmt.Fprintf(&b, "; conformal coverage target=%.0f%%", index.Calibration.Coverage*100)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if len(index.Candidates) == 0 {
		b.WriteString("No normative candidate statements were found. This is not evidence that the ADR corpus has no obligations.\n")
		return b.String()
	}
	b.WriteString("| Candidate | Confidence | Source | Section | Modality | Pattern hint | Escalation reasons | Statement | State |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, candidate := range index.Candidates {
		fmt.Fprintf(&b, "| %s | %s (p=%.2f) | %s:%d-%d | %s | %s | %s | %s | %s | %s / %s |\n",
			mdCell(candidate.ID), mdCell(candidate.Confidence), candidate.ConfidenceP,
			mdCell(candidate.SourcePath), candidate.SourceLineStart, candidate.SourceLineEnd,
			mdCell(candidate.SectionHeading), mdCell(candidate.Modality), mdCell(candidate.PatternHint),
			mdCell(strings.Join(candidate.EscalationReasons, ", ")), mdCell(candidate.Statement),
			mdCell(candidate.ApprovalStatus), mdCell(candidate.EnforcementMode))
	}
	return b.String()
}

// ExtractDocument accepts only source bytes matching indexed ID, title and SHA.
// It ignores fenced examples and records an exact original source line for each
// candidate; natural-language suggestions are deliberately not auto-enforced.
func ExtractDocument(doc adringest.Document, displayPath string, source []byte) ([]Candidate, error) {
	if len(source) > maxADRSourceBytes {
		return nil, fmt.Errorf("ADR source exceeds the %d-byte limit", maxADRSourceBytes)
	}
	parsed, err := adringest.Parse(doc.SourcePath, source)
	if err != nil {
		return nil, fmt.Errorf("reparse source ADR: %w", err)
	}
	if parsed.ID != doc.ID || parsed.Title != doc.Title || parsed.SourceSHA256 != doc.SourceSHA256 {
		return nil, fmt.Errorf("ADR source no longer matches the indexed ID, title, or SHA-256")
	}
	cleanDisplayPath, err := cleanRelativePath(displayPath)
	if err != nil {
		return nil, fmt.Errorf("candidate display path: %w", err)
	}
	lines := strings.Split(string(source), "\n")
	result := make([]Candidate, 0)
	// Normative statements may appear directly below the H1 title, especially in
	// free-form specifications without formal ADR headings. Treat this prefix as
	// a review-only preamble, stopping before the first parsed section heading.
	sections := append([]adringest.Section(nil), parsed.Sections...)
	preambleEnd := len(lines)
	if len(parsed.Sections) > 0 {
		preambleEnd = parsed.Sections[0].StartLine - 1
	}
	if preambleEnd > 1 {
		sections = append([]adringest.Section{{
			Heading:   "Preamble",
			Level:     0,
			StartLine: 1,
			EndLine:   preambleEnd,
		}}, sections...)
	} else if len(parsed.Sections) == 0 && len(lines) > 1 {
		sections = append([]adringest.Section{{
			Heading:   "Preamble",
			Level:     0,
			StartLine: 1,
			EndLine:   len(lines),
		}}, sections...)
	}
	for _, section := range sections {
		start, end := section.StartLine, section.EndLine
		if start < 1 || end < start || end > len(lines) {
			return nil, fmt.Errorf("section %q has invalid source line range %d-%d", section.Heading, start, end)
		}
		inFence := false
		for lineIndex := start; lineIndex < end; lineIndex++ {
			trimmed := strings.TrimSpace(lines[lineIndex])
			if isFenceLine(trimmed) {
				inFence = !inFence
				continue
			}
			if inFence || trimmed == "" || strings.HasPrefix(trimmed, "<!--") {
				continue
			}
			lineText := normalizeStatement(trimmed)
			if lineText == "" {
				continue
			}
			// Avoid expanding arbitrary prose into thousands of sentence fragments.
			// Preserve the original line-size bound before segmentation as well.
			if _, ok := detectModality(lineText); !ok {
				continue
			}
			if len(lineText) > maxCandidateLineBytes {
				return nil, fmt.Errorf("normative source line %d exceeds the %d-byte candidate statement limit", lineIndex+1, maxCandidateLineBytes)
			}
			expanded := expandNormativeStatements(lineText)
			for _, statement := range expanded {
				modality, ok := detectModality(statement)
				if !ok {
					continue
				}
				if len(statement) > maxCandidateLineBytes {
					return nil, fmt.Errorf("normative source line %d exceeds the %d-byte candidate statement limit", lineIndex+1, maxCandidateLineBytes)
				}
				kind := "normative_candidate"
				if anyPhrase(statement, "unless", "except", "only when", "other than", "waiver", "exception") {
					kind = "exception_candidate"
				}
				conditions, exceptions, ambiguity := classifyStatementStructure(statement)
				hints := reviewHints(statement, kind, section.Heading)
				if len(expanded) > 1 {
					hints = appendUniqueHint(hints, "compound_or_conjoined")
					hints = appendUniqueHint(hints, "split_from_compound")
				}
				if len(ambiguity) > 0 {
					hints = appendUniqueHint(hints, "ambiguous_interpretation")
				}
				candidate := Candidate{
					ADRID: parsed.ID, ADRTitle: parsed.Title, ADRStatus: parsed.Status,
					SectionHeading: section.Heading, Statement: statement, Modality: modality,
					Kind: kind, SubmissionKind: string(assessment.SubmissionKindTechSpec),
					PatternHint: suggestPattern(statement), SourcePath: cleanDisplayPath,
					SourceSHA256: parsed.SourceSHA256, SourceLineStart: lineIndex + 1, SourceLineEnd: lineIndex + 1,
					ApprovalStatus: "pending_review", EnforcementMode: "review_required", Executable: false,
					ReviewHints: hints, Conditions: conditions, Exceptions: exceptions, Ambiguity: ambiguity,
				}
				scored := ScoreCandidate(candidate)
				candidate.Confidence = scored.ConfidenceBand
				candidate.ConfidenceP = scored.ConfidenceP
				candidate.ConfidenceMethod = scored.ConfidenceMethod
				candidate.ConfidenceCoverage = scored.ConfidenceCoverage
				candidate.PredictionSet = scored.PredictionSet
				candidate.EscalationReasons = scored.EscalationReasons
				candidate.ID = candidateID(candidate)
				result = append(result, candidate)
			}
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
	type cue struct{ phrase, modality string }
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
		if index < 0 {
			continue
		}
		if bestIndex < 0 || index < bestIndex || (index == bestIndex && len(candidate.phrase) > bestLength) {
			bestIndex, bestLength, bestModality = index, len(candidate.phrase), candidate.modality
		}
	}
	if bestIndex < 0 {
		return "", false
	}
	return bestModality, true
}

// expandNormativeStatements splits a line into independently reviewable fragments.
// Sentence boundaries come first; compound clauses that each carry a modality are
// then separated so timeout+retry+backoff style requirements are not collapsed.
func expandNormativeStatements(line string) []string {
	out := make([]string, 0, 4)
	for _, sentence := range splitSentences(line) {
		parts := splitCompoundNormative(sentence)
		out = append(out, parts...)
	}
	return out
}

// splitCompoundNormative separates conjoined obligations when both sides contain
// a normative cue. Pure descriptive conjunctions stay intact.
func splitCompoundNormative(statement string) []string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return nil
	}
	separators := []string{"; ", ". "}
	for _, sep := range separators {
		if strings.Contains(statement, sep) {
			parts := strings.Split(statement, sep)
			usable := make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if _, ok := detectModality(part); ok {
					usable = append(usable, part)
				}
			}
			if len(usable) > 1 {
				return usable
			}
		}
	}
	// Split "X must ... . Y must ..." already handled. Also split "must A and must B".
	lower := strings.ToLower(statement)
	if idx := strings.Index(lower, " and "); idx > 0 {
		left, right := strings.TrimSpace(statement[:idx]), strings.TrimSpace(statement[idx+5:])
		_, leftOK := detectModality(left)
		_, rightOK := detectModality(right)
		if leftOK && rightOK {
			return []string{left, right}
		}
	}
	return []string{statement}
}

func classifyStatementStructure(statement string) (conditions, exceptions, ambiguity []string) {
	if anyPhrase(statement, "if", "when", "only when", "provided that", "wherever", "where") {
		conditions = append(conditions, "conditional_applicability")
	}
	if anyPhrase(statement, "unless", "except", "other than", "waiver", "exception", "only when a documented") {
		exceptions = append(exceptions, "exception_or_waiver_language")
	}
	if anyPhrase(statement, "prefer", "preferably", "ideally", "as appropriate", "as needed",
		"reasonable", "timely", "best effort", "sufficient", "adequate", "quickly", "securely", "flexible") {
		ambiguity = append(ambiguity, "subjective_or_preference_language")
	}
	if anyPhrase(statement, "should") && anyPhrase(statement, "must", "shall") {
		ambiguity = append(ambiguity, "conflicting_modality_cues")
	}
	if anyPhrase(statement, "attempts") && anyPhrase(statement, "retries", "retry") {
		ambiguity = append(ambiguity, "attempts_vs_retries_clarification")
	}
	return conditions, exceptions, ambiguity
}

func appendUniqueHint(hints []string, hint string) []string {
	for _, existing := range hints {
		if existing == hint {
			return hints
		}
	}
	return append(hints, hint)
}

// splitSentences separates independently reviewable prose sentences while keeping
// each fragment traceable to the same original source line. It splits only after
// sentence punctuation followed by whitespace, avoids common abbreviations and
// decimal numbers, and leaves comma/semicolon clauses intact because they may
// carry important scope or exception semantics.
func splitSentences(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	sentences := make([]string, 0, 2)
	start := 0
	for i := 0; i < len(line); i++ {
		punctuation := line[i]
		if punctuation != '.' && punctuation != '!' && punctuation != '?' {
			continue
		}
		end := i + 1
		if end < len(line) && line[end] != ' ' && line[end] != '\t' && line[end] != '\r' && line[end] != '\n' {
			continue
		}

		next := end
		for next < len(line) && (line[next] == ' ' || line[next] == '\t' || line[next] == '\r' || line[next] == '\n') {
			next++
		}
		if next >= len(line) {
			continue
		}

		prefix := strings.ToLower(strings.TrimSpace(line[start:end]))
		isAbbreviation := false
		if punctuation == '.' {
			for _, abbreviation := range sentenceAbbreviations {
				if strings.HasSuffix(prefix, abbreviation) {
					isAbbreviation = true
					break
				}
			}
		}
		if isAbbreviation {
			continue
		}

		sentence := strings.TrimSpace(line[start:end])
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
		start = next
		i = next - 1
	}
	if tail := strings.TrimSpace(line[start:]); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

// firstPhraseIndex returns the first case-insensitive whole-word occurrence.
func firstPhraseIndex(value, phrase string) int {
	lowerValue, lowerPhrase := strings.ToLower(value), strings.ToLower(phrase)
	offset := 0
	for offset <= len(lowerValue)-len(lowerPhrase) {
		idx := strings.Index(lowerValue[offset:], lowerPhrase)
		if idx < 0 {
			return -1
		}
		start := offset + idx
		end := start + len(lowerPhrase)
		leftOK := start == 0 || !isWordRune(runeAtEnd(lowerValue[:start]))
		rightOK := end == len(lowerValue) || !isWordRune(runeAtStart(lowerValue[end:]))
		if leftOK && rightOK {
			return start
		}
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
			if idx < 0 {
				break
			}
			start := offset + idx
			end := start + len(lowerPhrase)
			leftOK := start == 0 || !isWordRune(runeAtEnd(lowerText[:start]))
			rightOK := end == len(lowerText) || !isWordRune(runeAtStart(lowerText[end:]))
			if leftOK && rightOK {
				return true
			}
			offset = start + 1
		}
	}
	return false
}

func runeAtEnd(value string) rune {
	for _, r := range value {
		_ = r
	}
	runes := []rune(value)
	if len(runes) == 0 {
		return 0
	}
	return runes[len(runes)-1]
}

func runeAtStart(value string) rune {
	for _, r := range value {
		return r
	}
	return 0
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// reviewHints marks deterministic extraction risks for a human reviewer. Hints
// never change the candidate's review-required state and never activate a verifier.
func reviewHints(statement, kind, sectionHeading string) []string {
	lower := strings.ToLower(statement)
	hints := make([]string, 0, 5)
	if isNonDecisionSection(sectionHeading) {
		hints = append(hints, "non_decision_section")
	}
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
		"references", "appendix", "appendices", "preamble",
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
	case anyPhrase(lower, "timeout", "retry", "retries", "attempt", "attempts", "backoff", "configuration", "config", "required key", "setting", "retention", "rate-limit", "rate limit", "min-age", "max-age", "header", "tls minimum", "encryption-at-rest", "hop limit", "flow logs", "cloudtrail", "backup plan", "versioning", "block public access", "access analyzer", "kms cmk", "encryption by default"):
		return "configuration.bounds_or_required_values"
	case anyPhrase(lower, "shell", "command", "process.start", "runtime.exec", "eval", "credential", "secret", "injection", "sql", "ssrf", "url fetch", "deserialize", "password digest", "md5", "sha1", "plaintext", "public acl", "public-read", "access key", "xss", "ldap", "xpath", "nosql", "redirect follow", "link-local", "metadata exposure", "hardcoded", "hard-coded", "canned acl", "insecure deserialization", "unsigned update", "unsigned package", "cookie", "cors", "traversal", "privilege cookie", "mass-assignment", "template injection", "webhook", "macro", "cdn integrity", "mutable tag", "bearer token", "knowledge-based", "session fixation", "credential stuffing", "directory listing", "wildcard cors", "debug logging", "log shipping", "clock skew", "dns rebinding", "rfc1918", "file url", "stop-logging", "root access key", "root console", "user-data containing secrets", "elastic ip", "snapshot share", "sse-s3", "principal star", "vault policy", "vault lock", "inline iam", "external id", "world-open", "self-referencing"):
		return "security.source_pattern"
	case anyPhrase(lower, "dependency", "import", "package", "coupling", "gateway", "service boundary", "trust boundary", "authorization", "iam", "permission", "security group", "public subnet", "peering", "instance profile", "object-level", "function-level", "cross-tenant", "wildcard iam", "resource star", "direct object reference", "admin action", "tenancy boundary", "keypair", "public interface"):
		return "architecture.dependency_or_boundary"
	default:
		return "manual_review"
	}
}

func normalizeStatement(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
	trimmed := strings.TrimLeft(line, "-*+ ")
	if trimmed != line {
		line = strings.TrimSpace(trimmed)
	} else {
		i := 0
		for i < len(line) && line[i] >= '0' && line[i] <= '9' {
			i++
		}
		if i > 0 && i+1 < len(line) && (line[i] == '.' || line[i] == ')') && line[i+1] == ' ' {
			line = strings.TrimSpace(line[i+2:])
		}
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
	if root == "" {
		return ".", nil
	}
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
	if rootLabel == "" || rootLabel == "." {
		return clean
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(rootLabel), filepath.FromSlash(clean)))
}

func readContainedSource(root, sourcePath string) ([]byte, error) {
	relative, err := cleanRelativePath(sourcePath)
	if err != nil {
		return nil, err
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return nil, fmt.Errorf("resolve source file %q: %w", relative, err)
	}
	if filepath.Clean(candidate) != filepath.Clean(resolved) {
		return nil, fmt.Errorf("source file %q uses a symlinked path", relative)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return nil, fmt.Errorf("validate source containment: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("source path %q escapes the ADR root", relative)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("inspect source file %q: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source %q is not a regular file", relative)
	}
	if info.Size() > maxADRSourceBytes {
		return nil, fmt.Errorf("source %q exceeds the %d-byte limit", relative, maxADRSourceBytes)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read source file %q: %w", relative, err)
	}
	return data, nil
}

func cleanRelativePath(sourcePath string) (string, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(sourcePath)))
	unsafe := sourcePath == "" || strings.ContainsRune(sourcePath, 0) || filepath.IsAbs(filepath.FromSlash(clean)) ||
		clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || (len(clean) >= 2 && clean[1] == ':')
	if unsafe {
		return "", fmt.Errorf("source path %q must be a safe repository-relative path", sourcePath)
	}
	return clean, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
