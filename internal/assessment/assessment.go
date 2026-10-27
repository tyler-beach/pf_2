package assessment

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tyler-beach/pf_2/internal/analyzer"
)

type Catalog struct {
	SchemaVersion string `json:"schema_version"`
	ADRs []ADR `json:"adrs"`
}
type ADR struct {
	ID string `json:"id"`
	Title string `json:"title"`
	Version int `json:"version"`
	Status string `json:"status"`
	Requirements []Requirement `json:"requirements"`
}
type Requirement struct {
	ID string `json:"id"`
	Title string `json:"title"`
	Description string `json:"description"`
	Severity string `json:"severity"`
	Verifier Verifier `json:"verifier"`
}
type Verifier struct {
	Type string `json:"type"`
	SourcePathPrefixes []string `json:"source_path_prefixes"`
	ForbiddenImportPrefixes []string `json:"forbidden_import_prefixes"`
	FilePath string `json:"file_path,omitempty"`
	RequiredJSONKeys []string `json:"required_json_keys,omitempty"`
	NumericJSONBounds []JSONNumberBound `json:"numeric_json_bounds,omitempty"`
	Analyzer string `json:"analyzer,omitempty"`
	RuleIDs []string `json:"rule_ids,omitempty"`
}
type JSONNumberBound struct { Key string `json:"key"`; Min *float64 `json:"min,omitempty"`; Max *float64 `json:"max,omitempty"` }
type SourceFile struct {
	Path string `json:"path"`
	Language string `json:"language"`
	Content string `json:"-"`
}
type Finding struct {
	RuleID string `json:"rule_id"`
	Path string `json:"path"`
	Line int `json:"line"`
	Column int `json:"column,omitempty"`
	Message string `json:"message"`
	Analyzer string `json:"analyzer,omitempty"`
	AnalyzerVersion string `json:"analyzer_version,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Level string `json:"level,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}
type RequirementResult struct {
	ADR string `json:"adr_id"`
	ADRVersion int `json:"adr_version"`
	RequirementID string `json:"requirement_id"`
	Title string `json:"title"`
	Status string `json:"status"`
	Severity string `json:"severity"`
	Evidence []string `json:"evidence"`
	Findings []Finding `json:"findings"`
	Reason string `json:"reason,omitempty"`
}
type EvidenceMode string

const (
	EvidenceModeSubmission EvidenceMode = "submission"
	EvidenceModeRepository EvidenceMode = "repository"
)

// AssessmentOptions declares the completeness semantics of supplied evidence.
type AssessmentOptions struct {
	EvidenceMode EvidenceMode
	AnalyzerResults []analyzer.ScanResult
}

type Result struct {
	SchemaVersion string `json:"schema_version"`
	EvidenceMode string `json:"evidence_mode"`
	EngineVersion string `json:"engine_version"`
	Verdict string `json:"verdict"`
	CoverageStatus string `json:"coverage_status"`
	ApplicableRequirementCount int `json:"applicable_requirement_count"`
	SourceFileCount int `json:"source_file_count"`
	RequirementCount int `json:"requirement_count"`
	Counts map[string]int `json:"counts"`
	LLMCalls int `json:"runtime_llm_calls"`
	Requirements []RequirementResult `json:"requirements"`
	AnalyzerResults []analyzer.ScanResult `json:"analyzer_results,omitempty"`
	UnassessedSourceFiles []string `json:"unassessed_source_files,omitempty"`
}

func LoadCatalog(path string) (Catalog, error) {
	var c Catalog
	data, err := os.ReadFile(path)
	if err != nil { return c, fmt.Errorf("read catalog: %w", err) }
	if err := json.Unmarshal(data, &c); err != nil { return c, fmt.Errorf("decode catalog JSON: %w", err) }
	if err := ValidateCatalog(c); err != nil { return c, err }
	return c, nil
}

func ValidateCatalog(c Catalog) error {
	if c.SchemaVersion != "1" { return fmt.Errorf("unsupported SchemaVersion %q (expected 1)", c.SchemaVersion) }
	adrs, reqs := map[string]bool{}, map[string]bool{}
	for _, a := range c.ADRs {
		if a.ID == "" || a.Title == "" || a.Version < 1 { return fmt.Errorf("each ADR requires ID, Title, and Version >= 1") }
		if adrs[a.ID] { return fmt.Errorf("duplicate ADR ID %q", a.ID) }; adrs[a.ID] = true
		if a.Status != "accepted" && a.Status != "proposed" && a.Status != "superseded" { return fmt.Errorf("ADR %s has invalid Status %q", a.ID, a.Status) }
		for _, r := range a.Requirements {
			if r.ID == "" || r.Title == "" { return fmt.Errorf("ADR %s has requirement without ID or Title", a.ID) }
			if reqs[r.ID] { return fmt.Errorf("duplicate requirement ID %q", r.ID) }; reqs[r.ID] = true
			if r.Severity != "error" && r.Severity != "warning" && r.Severity != "info" { return fmt.Errorf("requirement %s has invalid Severity %q", r.ID, r.Severity) }
			if r.Verifier.Type == "" { return fmt.Errorf("requirement %s has no verifier Type", r.ID) }
			switch r.Verifier.Type {
			case "go-import-boundary":
				if len(r.Verifier.SourcePathPrefixes) == 0 { return fmt.Errorf("requirement %s: go-import-boundary requires source_path_prefixes", r.ID) }
				if len(r.Verifier.ForbiddenImportPrefixes) == 0 { return fmt.Errorf("requirement %s: go-import-boundary requires forbidden_import_prefixes", r.ID) }
			case "json-number-range":
				p := filepath.Clean(filepath.FromSlash(strings.TrimSpace(r.Verifier.FilePath)))
				if r.Verifier.FilePath == "" || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) { return fmt.Errorf("requirement %s: json-number-range requires a repository-relative file_path", r.ID) }
				if len(r.Verifier.NumericJSONBounds) == 0 { return fmt.Errorf("requirement %s: json-number-range requires numeric_json_bounds", r.ID) }
				for _, b := range r.Verifier.NumericJSONBounds {
					if strings.TrimSpace(b.Key) == "" { return fmt.Errorf("requirement %s: numeric_json_bounds keys cannot be empty", r.ID) }
					if b.Min == nil && b.Max == nil { return fmt.Errorf("requirement %s: numeric_json_bounds for %q needs min and/or max", r.ID, b.Key) }
					if b.Min != nil && b.Max != nil && *b.Min > *b.Max { return fmt.Errorf("requirement %s: numeric_json_bounds for %q has min greater than max", r.ID, b.Key) }
				}
			case "analyzer-rule":
				if strings.TrimSpace(r.Verifier.Analyzer) == "" { return fmt.Errorf("requirement %s: analyzer-rule requires analyzer", r.ID) }
				if len(r.Verifier.RuleIDs) == 0 { return fmt.Errorf("requirement %s: analyzer-rule requires rule_ids", r.ID) }
				if len(r.Verifier.SourcePathPrefixes) == 0 { return fmt.Errorf("requirement %s: analyzer-rule requires source_path_prefixes", r.ID) }
				seenRules := map[string]bool{}
				for _, ruleID := range r.Verifier.RuleIDs {
					if strings.TrimSpace(ruleID) == "" { return fmt.Errorf("requirement %s: analyzer rule IDs cannot be empty", r.ID) }
					if seenRules[ruleID] { return fmt.Errorf("requirement %s: duplicate analyzer rule ID %q", r.ID, ruleID) }
					seenRules[ruleID] = true
				}
				for _, prefix := range r.Verifier.SourcePathPrefixes {
					p := filepath.Clean(filepath.FromSlash(strings.TrimSpace(prefix)))
					if prefix == "" || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
						return fmt.Errorf("requirement %s: analyzer source_path_prefixes must be repository-relative", r.ID)
					}
				}
			case "json-required-keys":
				p := filepath.Clean(filepath.FromSlash(strings.TrimSpace(r.Verifier.FilePath)))
				if r.Verifier.FilePath == "" || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
					return fmt.Errorf("requirement %s: json-required-keys requires a repository-relative file_path", r.ID)
				}
				if len(r.Verifier.RequiredJSONKeys) == 0 { return fmt.Errorf("requirement %s: json-required-keys requires required_json_keys", r.ID) }
				for _, key := range r.Verifier.RequiredJSONKeys { if strings.TrimSpace(key) == "" { return fmt.Errorf("requirement %s: required_json_keys cannot contain empty keys", r.ID) } }
			}
		}
	}
	return nil
}

func ParseIssueMarkdown(markdown string) []SourceFile {
	lines := strings.Split(markdown, "\n")
	fence := strings.Repeat(string(rune(96)), 3)
	var files []SourceFile
	currentPath := ""
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "### File:") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "### File:"))
			continue
		}
		if !strings.HasPrefix(line, fence) { continue }
		info := strings.TrimSpace(strings.TrimPrefix(line, fence))
		fields := strings.Fields(info)
		declaredLanguage := ""
		if len(fields) > 0 { declaredLanguage = strings.ToLower(strings.TrimSpace(fields[0])) }
		var code []string
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
			code = append(code, lines[i])
			i++
		}
		language, extension, recognized := normalizeSourceLanguage(declaredLanguage, currentPath)
		if !recognized {
			if declaredLanguage == "" { declaredLanguage = "unknown" }
			language = "unsupported:" + sanitizeLanguageLabel(declaredLanguage)
			extension = filepath.Ext(currentPath)
			if extension == "" || strings.ContainsAny(extension, "/\\") { extension = ".txt" }
		}
		sourcePath := safeSubmissionPath(currentPath, len(files)+1, extension)
		files = append(files, SourceFile{Path: sourcePath, Language: language, Content: strings.Join(code, "\n")})
	}
	return files
}

func normalizeSourceLanguage(declared, sourcePath string) (language, extension string, recognized bool) {
	type languageSpec struct { language, extension string }
	aliases := map[string]languageSpec{
		"go": {"go", ".go"}, "golang": {"go", ".go"},
		"json": {"json", ".json"},
		"python": {"python", ".py"}, "py": {"python", ".py"}, "python3": {"python", ".py"},
		"javascript": {"javascript", ".js"}, "js": {"javascript", ".js"}, "jsx": {"javascript", ".jsx"},
		"mjs": {"javascript", ".mjs"}, "cjs": {"javascript", ".cjs"},
		"typescript": {"typescript", ".ts"}, "ts": {"typescript", ".ts"}, "tsx": {"typescript", ".tsx"},
		"java": {"java", ".java"},
		"csharp": {"csharp", ".cs"}, "c#": {"csharp", ".cs"}, "cs": {"csharp", ".cs"}, "dotnet": {"csharp", ".cs"},
		"php": {"php", ".php"},
		"ruby": {"ruby", ".rb"}, "rb": {"ruby", ".rb"},
		"rust": {"rust", ".rs"}, "rs": {"rust", ".rs"},
		"bash": {"shell", ".sh"}, "sh": {"shell", ".sh"}, "shell": {"shell", ".sh"},
		"yaml": {"yaml", ".yml"}, "yml": {"yaml", ".yml"},
		"hcl": {"hcl", ".tf"}, "terraform": {"hcl", ".tf"}, "tf": {"hcl", ".tf"},
		"kotlin": {"kotlin", ".kt"}, "kt": {"kotlin", ".kt"}, "scala": {"scala", ".scala"},
		"c": {"c", ".c"}, "cpp": {"cpp", ".cpp"}, "c++": {"cpp", ".cpp"}, "cxx": {"cpp", ".cpp"},
		"sql": {"sql", ".sql"}, "html": {"html", ".html"}, "xml": {"xml", ".xml"}, "vue": {"vue", ".vue"},
	}
	if declared != "" {
		if item, ok := aliases[declared]; ok { return item.language, item.extension, true }
	}
	extensions := map[string]languageSpec{
		".go": {"go", ".go"}, ".json": {"json", ".json"}, ".py": {"python", ".py"},
		".js": {"javascript", ".js"}, ".jsx": {"javascript", ".jsx"}, ".mjs": {"javascript", ".mjs"},
		".cjs": {"javascript", ".cjs"}, ".ts": {"typescript", ".ts"}, ".tsx": {"typescript", ".tsx"},
		".java": {"java", ".java"}, ".cs": {"csharp", ".cs"}, ".php": {"php", ".php"},
		".rb": {"ruby", ".rb"}, ".rs": {"rust", ".rs"}, ".sh": {"shell", ".sh"},
		".yaml": {"yaml", ".yml"}, ".yml": {"yaml", ".yml"}, ".tf": {"hcl", ".tf"},
		".hcl": {"hcl", ".tf"}, ".kt": {"kotlin", ".kt"}, ".kts": {"kotlin", ".kts"},
		".scala": {"scala", ".scala"}, ".c": {"c", ".c"}, ".h": {"c", ".h"},
		".cc": {"cpp", ".cc"}, ".cpp": {"cpp", ".cpp"}, ".hpp": {"cpp", ".hpp"},
		".sql": {"sql", ".sql"}, ".html": {"html", ".html"}, ".xml": {"xml", ".xml"}, ".vue": {"vue", ".vue"},
	}
	if item, ok := extensions[strings.ToLower(filepath.Ext(sourcePath))]; ok {
		return item.language, item.extension, true
	}
	return "", ".txt", false
}

func sanitizeLanguageLabel(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	var b strings.Builder
	for _, r := range language {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else { b.WriteByte('_') }
	}
	if b.Len() == 0 { return "unknown" }
	return b.String()
}

// safeSubmissionPath prevents untrusted issue headings from escaping a logical
// repository-relative path or injecting Markdown/report syntax through a finding path.
func safeSubmissionPath(raw string, index int, extension string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	clean := filepath.ToSlash(filepath.Clean(raw))
	unsafe := raw == "" || raw == "." || filepath.IsAbs(filepath.FromSlash(clean)) ||
		clean == ".." || strings.HasPrefix(clean, "../") || (len(clean) >= 2 && clean[1] == ':')
	if unsafe { return fmt.Sprintf("issue-snippet-%d%s", index, extension) }
	var b strings.Builder
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("._/-", r) {
			b.WriteRune(r)
		} else { b.WriteByte('_') }
	}
	result := strings.TrimPrefix(b.String(), "./")
	if result == "" || result == "." || strings.HasPrefix(result, "../") {
		return fmt.Sprintf("issue-snippet-%d%s", index, extension)
	}
	return result
}

const maxSourceFileBytes int64 = 4 << 20

func LoadSourceFiles(root string) ([]SourceFile, error) {
	var files []SourceFile
	rootAbs, err := filepath.Abs(root)
	if err != nil { return nil, fmt.Errorf("resolve repository root: %w", err) }
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil { return nil, fmt.Errorf("resolve repository root symlinks: %w", err) }
	rootInfo, err := os.Stat(rootReal)
	if err != nil { return nil, fmt.Errorf("inspect repository root: %w", err) }
	if !rootInfo.IsDir() { return nil, fmt.Errorf("repository root %q is not a directory", rootReal) }
	root = rootReal
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if d.Type()&os.ModeSymlink != 0 { return nil }
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata" || name == "analyzer-evidence" || name == ".opengrep-cache") { return filepath.SkipDir }
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" && ext != ".json" { return nil }
		rel, err := filepath.Rel(root, path)
		if err != nil { return err }
		relative := filepath.ToSlash(rel)
		if relative == "assessment.json" || relative == "adr/ingested-adrs.json" { return nil }
		info, err := d.Info()
		if err != nil { return fmt.Errorf("inspect source evidence %s: %w", relative, err) }
		if !info.Mode().IsRegular() { return nil }
		if info.Size() > maxSourceFileBytes {
			return fmt.Errorf("source evidence file %s exceeds the %d-byte limit", relative, maxSourceFileBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil { return fmt.Errorf("read source evidence %s: %w", relative, err) }
		language := "go"
		if ext == ".json" { language = "json" }
		files = append(files, SourceFile{Path: relative, Language: language, Content: string(data)})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func Assess(c Catalog, files []SourceFile) Result {
	return AssessWithOptions(c, files, AssessmentOptions{EvidenceMode: EvidenceModeSubmission})
}

// AssessWithOptions evaluates requirements without treating absent evidence as compliance.
func AssessWithOptions(c Catalog, files []SourceFile, options AssessmentOptions) Result {
	if options.EvidenceMode == "" {
		options.EvidenceMode = EvidenceModeSubmission
	}
	out := Result{SchemaVersion:"1", EvidenceMode:string(options.EvidenceMode), EngineVersion:"0.3.0", SourceFileCount:len(files), Counts:map[string]int{}, LLMCalls:0, AnalyzerResults:options.AnalyzerResults}
	if options.EvidenceMode == EvidenceModeSubmission {
		for _, source := range files {
			language := strings.ToLower(strings.TrimSpace(source.Language))
			if language != "go" && language != "json" {
				if language == "" { language = "unknown" }
				out.UnassessedSourceFiles = append(out.UnassessedSourceFiles, fmt.Sprintf("%s (%s)", source.Path, language))
			}
		}
		sort.Strings(out.UnassessedSourceFiles)
	}
	for _, a := range c.ADRs {
		if a.Status != "accepted" { continue }
		for _, r := range a.Requirements {
			rr := RequirementResult{ADR:a.ID, ADRVersion:a.Version, RequirementID:r.ID, Title:r.Title, Severity:r.Severity, Evidence:[]string{}, Findings:[]Finding{}}
			switch r.Verifier.Type {
			case "go-import-boundary": assessImportBoundary(&rr, r.Verifier, files)
			case "json-required-keys": assessJSONRequiredKeys(&rr, r.Verifier, files)
			case "json-number-range": assessJSONNumberRange(&rr, r.Verifier, files)
			case "analyzer-rule":
				if options.EvidenceMode == EvidenceModeSubmission && len(options.AnalyzerResults) == 0 {
					rr.Status = "NOT_APPLICABLE"
					rr.Reason = "Repository analyzer rules are not run against issue-submitted snippets; assess a repository snapshot to evaluate this rule."
				} else { assessAnalyzerRule(&rr, r.Verifier, options.AnalyzerResults) }
			default: rr.Status="ERROR"; rr.Reason="Unsupported verifier type: "+r.Verifier.Type
			}
			out.Counts[rr.Status]++
			out.Requirements = append(out.Requirements, rr)
		}
	}
	out.RequirementCount = len(out.Requirements)
	for _, rr := range out.Requirements { if rr.Status == "PASS" || rr.Status == "FAIL" { out.ApplicableRequirementCount++ } }
	if out.RequirementCount == 0 || out.ApplicableRequirementCount == 0 { out.CoverageStatus = "NONE" } else if out.ApplicableRequirementCount != out.RequirementCount { out.CoverageStatus = "PARTIAL" } else { out.CoverageStatus = "COMPLETE" }
	out.Verdict = rollup(out.Requirements)
	if len(out.UnassessedSourceFiles) > 0 {
		if out.CoverageStatus != "NONE" { out.CoverageStatus = "PARTIAL" }
		if out.Verdict != "FAIL" { out.Verdict = "INDETERMINATE" }
	}
	return out
}

func assessImportBoundary(rr *RequirementResult, v Verifier, files []SourceFile) {
	if len(files) == 0 { rr.Status="EVIDENCE_MISSING"; rr.Reason="No source files supplied or discovered; absence of evidence is not compliance."; return }
	var scoped []SourceFile
	for _, f := range files { if f.Language == "go" && pathMatches(f.Path, v.SourcePathPrefixes) { scoped=append(scoped,f) } }
	if len(scoped) == 0 { rr.Status="NOT_APPLICABLE"; rr.Reason="No supplied Go files matched the declared source path scope."; return }
	for _, f := range scoped {
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, f.Path, f.Content, parser.AllErrors)
		if err != nil { rr.Status="ERROR"; rr.Reason=fmt.Sprintf("Could not parse %s: %v", f.Path, err); return }
		rr.Evidence = append(rr.Evidence, fmt.Sprintf("Parsed %s and inspected %d import declarations.", f.Path, len(parsed.Imports)))
		for _, imp := range parsed.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil { rr.Status="ERROR"; rr.Reason="Could not decode import in "+f.Path; return }
			for _, forbidden := range v.ForbiddenImportPrefixes {
				if importMatches(p, forbidden) {
					pos := fset.Position(imp.Pos())
					rr.Findings = append(rr.Findings, Finding{RuleID:rr.RequirementID, Path:f.Path, Line:pos.Line, Message:fmt.Sprintf("Import %q violates %s (forbidden prefix %q).",p,rr.RequirementID,forbidden)})
					break
				}
			}
		}
	}
	if len(rr.Findings)>0 { rr.Status="FAIL"; rr.Reason=fmt.Sprintf("Found %d forbidden import(s).",len(rr.Findings)) } else { rr.Status="PASS"; rr.Reason=fmt.Sprintf("All %d in-scope Go file(s) parsed and no forbidden imports were found.",len(scoped)) }
}

func assessJSONRequiredKeys(rr *RequirementResult, v Verifier, files []SourceFile) {
	if strings.TrimSpace(v.FilePath) == "" || len(v.RequiredJSONKeys) == 0 {
		rr.Status = "ERROR"
		rr.Reason = "json-required-keys requires file_path and at least one required_json_keys entry."
		return
	}
	var target *SourceFile
	for i := range files {
		if files[i].Language == "json" && filepath.ToSlash(files[i].Path) == filepath.ToSlash(v.FilePath) {
			target = &files[i]
			break
		}
	}
	if target == nil {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("Required configuration file %s was not included in the supplied evidence.", v.FilePath)
		return
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(target.Content), &document); err != nil {
		rr.Status = "ERROR"
		rr.Reason = fmt.Sprintf("Could not parse JSON file %s: %v", target.Path, err)
		return
	}
	rr.Evidence = append(rr.Evidence, fmt.Sprintf("Parsed JSON document %s and inspected required top-level keys.", target.Path))
	for _, key := range v.RequiredJSONKeys {
		if _, ok := document[key]; !ok {
			rr.Findings = append(rr.Findings, Finding{
				RuleID: rr.RequirementID,
				Path: target.Path,
				Line: 1,
				Message: fmt.Sprintf("Required JSON key %q is missing from %s.", key, target.Path),
			})
		}
	}
	if len(rr.Findings) > 0 {
		rr.Status = "FAIL"
		rr.Reason = fmt.Sprintf("Missing %d required JSON key(s).", len(rr.Findings))
	} else {
		rr.Status = "PASS"
		rr.Reason = fmt.Sprintf("All %d required JSON key(s) are present in %s.", len(v.RequiredJSONKeys), target.Path)
	}
}


func assessJSONNumberRange(rr *RequirementResult, v Verifier, files []SourceFile) {
	if strings.TrimSpace(v.FilePath) == "" || len(v.NumericJSONBounds) == 0 {
		rr.Status = "ERROR"
		rr.Reason = "json-number-range requires file_path and at least one numeric_json_bounds entry."
		return
	}
	var target *SourceFile
	for i := range files {
		if files[i].Language == "json" && filepath.ToSlash(files[i].Path) == filepath.ToSlash(v.FilePath) {
			target = &files[i]
			break
		}
	}
	if target == nil {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("Required configuration file %s was not included in the supplied evidence.", v.FilePath)
		return
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(target.Content), &document); err != nil {
		rr.Status = "ERROR"
		rr.Reason = fmt.Sprintf("Could not parse JSON file %s: %v", target.Path, err)
		return
	}
	rr.Evidence = append(rr.Evidence, fmt.Sprintf("Parsed JSON document %s and checked numeric bounds.", target.Path))
	for _, bound := range v.NumericJSONBounds {
		value, exists := document[bound.Key]
		if !exists {
			rr.Findings = append(rr.Findings, Finding{RuleID: rr.RequirementID, Path: target.Path, Line: 1, Message: fmt.Sprintf("Required numeric JSON key %q is missing from %s.", bound.Key, target.Path)})
			continue
		}
		number, ok := value.(float64)
		if !ok {
			rr.Findings = append(rr.Findings, Finding{RuleID: rr.RequirementID, Path: target.Path, Line: 1, Message: fmt.Sprintf("JSON key %q must be a number; found %T.", bound.Key, value)})
			continue
		}
		if bound.Min != nil && number < *bound.Min {
			rr.Findings = append(rr.Findings, Finding{RuleID: rr.RequirementID, Path: target.Path, Line: 1, Message: fmt.Sprintf("JSON key %q is %v, below minimum %v.", bound.Key, number, *bound.Min)})
		}
		if bound.Max != nil && number > *bound.Max {
			rr.Findings = append(rr.Findings, Finding{RuleID: rr.RequirementID, Path: target.Path, Line: 1, Message: fmt.Sprintf("JSON key %q is %v, above maximum %v.", bound.Key, number, *bound.Max)})
		}
	}
	if len(rr.Findings) > 0 {
		rr.Status = "FAIL"
		rr.Reason = fmt.Sprintf("Found %d numeric JSON constraint violation(s).", len(rr.Findings))
	} else {
		rr.Status = "PASS"
		rr.Reason = fmt.Sprintf("All %d numeric JSON constraint(s) passed in %s.", len(v.NumericJSONBounds), target.Path)
	}
}

func assessAnalyzerRule(rr *RequirementResult, verifier Verifier, scans []analyzer.ScanResult) {
	var matches []analyzer.ScanResult
	want := strings.ToLower(strings.TrimSpace(verifier.Analyzer))
	for _, scan := range scans {
		for _, info := range scan.Analyzers {
			name := strings.ToLower(strings.TrimSpace(info.Name))
			if name == want || strings.Contains(name, want) { matches = append(matches, scan); break }
		}
	}
	if len(matches) == 0 {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("No normalized scan evidence was supplied for analyzer %q.", verifier.Analyzer)
		return
	}
	if len(matches) != 1 {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("Expected exactly one scan result for analyzer %q; found %d, so the evidence is ambiguous.", verifier.Analyzer, len(matches))
		return
	}
	scan := matches[0]
	if scan.Status != analyzer.ScanStatusComplete {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("Analyzer %q did not produce complete evidence (status %s). %s", verifier.Analyzer, scan.Status, strings.Join(scan.Errors, "; "))
		return
	}
	if scan.InputSHA256 == "" || scan.ConfigurationSHA256 == "" || scan.RawOutputSHA256 == "" {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("Analyzer %q evidence is missing input, configuration, or raw-output SHA-256 provenance.", verifier.Analyzer)
		return
	}
	var scopedPaths []string
	scannedPathSet := make(map[string]bool, len(scan.ScannedPaths))
	for _, sourcePath := range scan.ScannedPaths {
		normalizedPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(sourcePath)))
		scannedPathSet[normalizedPath] = true
		if pathMatches(sourcePath, verifier.SourcePathPrefixes) { scopedPaths = append(scopedPaths, normalizedPath) }
	}
	if len(scopedPaths) == 0 {
		rr.Status = "EVIDENCE_MISSING"
		rr.Reason = fmt.Sprintf("The completed %q scan inventory contains no source files in required scope %s; absence of scoped evidence is not compliance.", verifier.Analyzer, strings.Join(verifier.SourcePathPrefixes, ", "))
		return
	}
	version := ""
	for _, info := range scan.Analyzers {
		name := strings.ToLower(strings.TrimSpace(info.Name))
		if name == want || strings.Contains(name, want) {
			if info.Version != "" {
				if version != "" && version != info.Version {
					rr.Status = "EVIDENCE_MISSING"
					rr.Reason = fmt.Sprintf("Analyzer %q evidence contains conflicting versions.", verifier.Analyzer)
					return
				}
				version = info.Version
			}
		}
	}
	rr.Evidence = append(rr.Evidence, fmt.Sprintf("Analyzer %s version %s completed against input SHA-256 %s with configuration SHA-256 %s and raw-output SHA-256 %s; %d discovered source path(s) matched scope.", verifier.Analyzer, version, scan.InputSHA256, scan.ConfigurationSHA256, scan.RawOutputSHA256, len(scopedPaths)))
	ruleIDs := make(map[string]bool, len(verifier.RuleIDs))
	for _, id := range verifier.RuleIDs { ruleIDs[id] = true }
	for _, finding := range scan.Findings {
		if !ruleIDs[finding.RuleID] { continue }
		if strings.TrimSpace(finding.Path) == "" {
			rr.Status = "EVIDENCE_MISSING"
			rr.Reason = fmt.Sprintf("Analyzer %q returned matching rule %q without a repository path; scope cannot be verified.", verifier.Analyzer, finding.RuleID)
			return
		}
		if !pathMatches(finding.Path, verifier.SourcePathPrefixes) { continue }
		findingPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(finding.Path)))
		if !scannedPathSet[findingPath] {
			rr.Status = "EVIDENCE_MISSING"
			rr.Reason = fmt.Sprintf("Analyzer %q returned matching rule %q for path %q that is absent from the verified source inventory.", verifier.Analyzer, finding.RuleID, finding.Path)
			return
		}
		rr.Findings = append(rr.Findings, Finding{
			RuleID:finding.RuleID, Path:filepath.ToSlash(finding.Path), Line:finding.StartLine, Column:finding.StartColumn,
			Message:finding.Message, Analyzer:finding.Analyzer, AnalyzerVersion:finding.AnalyzerVersion,
			Fingerprint:finding.Fingerprint, Level:finding.Level, Properties:finding.Properties,
		})
	}
	if len(rr.Findings) > 0 {
		rr.Status = "FAIL"
		rr.Reason = fmt.Sprintf("Analyzer %q found %d scoped violation(s) for rule(s) %s.", verifier.Analyzer, len(rr.Findings), strings.Join(verifier.RuleIDs, ", "))
	} else {
		rr.Status = "PASS"
		rr.Reason = fmt.Sprintf("No finding matched rule(s) %s within %d source path(s) in scope.", strings.Join(verifier.RuleIDs, ", "), len(scopedPaths))
	}
}

func pathMatches(path string, prefixes []string) bool {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	path = strings.TrimPrefix(path, "./")
	if path == "" || path == "." || path == ".." || strings.HasPrefix(path, "../") { return false }
	for _, prefix := range prefixes {
		p := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(prefix))))
		p = strings.TrimPrefix(p, "./")
		if p == "" { continue }
		if p == "." { return true }
		p = strings.TrimSuffix(p, "/")
		if path == p || strings.HasPrefix(path, p+"/") { return true }
	}
	return false
}

func importMatches(p, forbidden string) bool {
	forbidden=strings.TrimSuffix(strings.TrimSpace(forbidden),"/")
	return forbidden!="" && (p==forbidden || strings.HasPrefix(p,forbidden+"/"))
}
func rollup(rs []RequirementResult) string {
	if len(rs)==0 { return "INDETERMINATE" }
	passed:=0
	for _, r := range rs { switch r.Status {
		case "FAIL": return "FAIL"
		case "ERROR", "INDETERMINATE", "EVIDENCE_MISSING", "UNSUPPORTED", "REVIEW_REQUIRED": return "INDETERMINATE"
		case "PASS": passed++
		case "NOT_APPLICABLE":
		default: return "INDETERMINATE"
	} }
	if passed==0 { return "INDETERMINATE" }
	return "PASS"
}
