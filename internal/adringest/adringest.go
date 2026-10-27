// Package adringest parses ADR Markdown into deterministic, provenance-preserving
// documents. It does not infer enforceable obligations or execute document content.
package adringest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var (
	adrIDPattern = regexp.MustCompile(`(?i)\bADR[-_ ]?([0-9]+)\b`)
	statusPattern = regexp.MustCompile(`(?i)^\s*(?:[-*]\s*)?status\s*:\s*(.*?)\s*$`)
)

// Document is the parsed, immutable view of one source ADR. SourceSHA256 is
// computed over the exact input bytes so downstream artifacts can be audited.
type Document struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status,omitempty"`
	SourcePath  string    `json:"source_path"`
	SourceSHA256 string   `json:"source_sha256"`
	Sections    []Section `json:"sections"`
	DecisionText string   `json:"decision_text,omitempty"`
}

// Section retains the heading level and 1-based source line range for traceability.
type Section struct {
	Heading   string `json:"heading"`
	Level     int    `json:"level"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

type headingInfo struct {
	title string
	level int
	line  int // zero-based line index
}

// Parse parses one ADR Markdown document. The source path must be a
// repository-relative path; source is treated as data and never executed.
func Parse(sourcePath string, source []byte) (Document, error) {
	var zero Document
	normalizedPath, err := cleanSourcePath(sourcePath)
	if err != nil {
		return zero, err
	}
	if len(strings.TrimSpace(string(source))) == 0 {
		return zero, errors.New("ADR document is empty")
	}

	root := goldmark.New().Parser().Parse(text.NewReader(source))
	lines := strings.Split(string(source), "\n")
	headings := make([]headingInfo, 0, 8)
	err = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		startLine := 0
		segments := h.Lines()
		if segments != nil && segments.Len() > 0 {
			startLine = lineIndexAtOffset(source, segments.At(0).Start)
		} else {
			startLine = findHeadingLine(lines, string(h.Text(source)), len(headings))
		}
		headings = append(headings, headingInfo{
			title: strings.TrimSpace(string(h.Text(source))),
			level: h.Level,
			line:  startLine,
		})
		return ast.WalkContinue, nil
	})
	if err != nil {
		return zero, fmt.Errorf("walk ADR Markdown AST: %w", err)
	}
	if len(headings) == 0 || headings[0].level != 1 {
		return zero, errors.New("ADR document must contain a level-one title heading")
	}

	title := headings[0].title
	id := inferID(title, normalizedPath)
	if id == "" {
		return zero, errors.New("could not derive a stable ADR ID from title or filename")
	}
	doc := Document{
		ID:           id,
		Title:        title,
		SourcePath:   normalizedPath,
		SourceSHA256: hashSource(source),
		Sections:     make([]Section, 0, len(headings)-1),
	}
	for i := 1; i < len(headings); i++ {
		h := headings[i]
		if h.level == 1 {
			continue
		}
		start := clamp(h.line, 0, len(lines))
		end := len(lines)
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= h.level {
				end = clamp(headings[j].line, start, len(lines))
				break
			}
		}
		bodyStart := start + 1
		if bodyStart > end {
			bodyStart = end
		}
		sectionText := strings.TrimSpace(strings.Join(lines[bodyStart:end], "\n"))
		section := Section{
			Heading:   h.title,
			Level:     h.level,
			StartLine: start + 1,
			EndLine:   end,
			Text:      sectionText,
		}
		doc.Sections = append(doc.Sections, section)
		if strings.EqualFold(strings.TrimSpace(h.title), "decision") {
			doc.DecisionText = sectionText
		}
	}
	doc.Status = statusFromSections(doc.Sections)
	if doc.Status == "" {
		doc.Status = statusFromLegacyLines(lines)
	}
	return doc, nil
}

// ParseDir recursively parses Markdown files under root in lexical path order.
// It fails closed if any Markdown file cannot be parsed; callers can therefore
// avoid silently ingesting only part of an ADR corpus.
func ParseDir(root string) ([]Document, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat ADR root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("ADR root %q is not a directory", root)
	}
	var paths []string
	err = filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filePath != root {
				switch entry.Name() {
				case ".git", "vendor", "node_modules":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !entry.Type().IsRegular() && entry.Type() != 0 {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			paths = append(paths, filePath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk ADR root %q: %w", root, err)
	}
	sort.Strings(paths)
	docs := make([]Document, 0, len(paths))
	for _, filePath := range paths {
		source, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("read ADR %q: %w", filePath, readErr)
		}
		rel, relErr := filepath.Rel(root, filePath)
		if relErr != nil {
			return nil, fmt.Errorf("make ADR path relative: %w", relErr)
		}
		doc, parseErr := Parse(filepath.ToSlash(rel), source)
		if parseErr != nil {
			return nil, fmt.Errorf("parse ADR %q: %w", filepath.ToSlash(rel), parseErr)
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func cleanSourcePath(sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("source path is required")
	}
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(sourcePath)))
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source path must be repository-relative: %q", sourcePath)
	}
	return path.Clean(filepath.ToSlash(cleaned)), nil
}

func inferID(title, sourcePath string) string {
	if match := adrIDPattern.FindString(title); match != "" {
		match = strings.ToUpper(strings.NewReplacer("_", "-", " ", "-").Replace(match))
		if strings.HasPrefix(match, "ADR-") {
			return match
		}
		return "ADR-" + strings.TrimPrefix(match, "ADR")
	}
	base := strings.TrimSuffix(path.Base(sourcePath), path.Ext(sourcePath))
	return strings.TrimSpace(base)
}

func statusFromSections(sections []Section) string {
	for _, section := range sections {
		if !strings.EqualFold(strings.TrimSpace(section.Heading), "status") {
			continue
		}
		for _, line := range strings.Split(section.Text, "\n") {
			if strings.TrimSpace(line) != "" {
				return normalizeStatus(strings.TrimSpace(strings.TrimLeft(line, "-* ")))
			}
		}
	}
	return ""
}

func statusFromLegacyLines(lines []string) string {
	inFence := false
	fenceMarker := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if !inFence {
				inFence, fenceMarker = true, marker
			} else if marker == fenceMarker {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		if match := statusPattern.FindStringSubmatch(line); len(match) == 2 {
			return normalizeStatus(match[1])
		}
	}
	return ""
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.Trim(status, "`*_ ")
	if i := strings.IndexAny(status, "\r\n"); i >= 0 {
		status = status[:i]
	}
	return status
}

func hashSource(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

func lineIndexAtOffset(source []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	return strings.Count(string(source[:offset]), "\n")
}

func findHeadingLine(lines []string, title string, from int) int {
	for i := clamp(from, 0, len(lines)); i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		line = strings.TrimLeft(line, "#")
		if strings.TrimSpace(line) == title {
			return i
		}
	}
	return clamp(from, 0, max(0, len(lines)-1))
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
