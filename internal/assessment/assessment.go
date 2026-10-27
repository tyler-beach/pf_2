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
	Message string `json:"message"`
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
type Result struct {
	SchemaVersion string `json:"schema_version"`
	EngineVersion string `json:"engine_version"`
	Verdict string `json:"verdict"`
	CoverageStatus string `json:"coverage_status"`
	ApplicableRequirementCount int `json:"applicable_requirement_count"`
	SourceFileCount int `json:"source_file_count"`
	RequirementCount int `json:"requirement_count"`
	Counts map[string]int `json:"counts"`
	LLMCalls int `json:"runtime_llm_calls"`
	Requirements []RequirementResult `json:"requirements"`
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
	fence := strings.Repeat("`", 3)
	var files []SourceFile
	currentPath := ""
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "### File:") { currentPath = strings.TrimSpace(strings.TrimPrefix(line, "### File:")); continue }
		if !strings.HasPrefix(line, fence) { continue }
		info := strings.TrimSpace(strings.TrimPrefix(line, fence))
		fields := strings.Fields(info)
		language := ""
		if len(fields) > 0 { language = strings.ToLower(fields[0]) }
		var code []string
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), fence) { code = append(code, lines[i]); i++ }
		if language == "go" || strings.HasSuffix(strings.ToLower(currentPath), ".go") {
			path := currentPath
			if path == "" { path = fmt.Sprintf("issue-snippet-%d.go", len(files)+1) }
			files = append(files, SourceFile{Path: filepath.ToSlash(path), Language: "go", Content: strings.Join(code, "\n")})
		} else if language == "json" || strings.HasSuffix(strings.ToLower(currentPath), ".json") {
			path := currentPath
			if path == "" { path = fmt.Sprintf("issue-snippet-%d.json", len(files)+1) }
			files = append(files, SourceFile{Path: filepath.ToSlash(path), Language: "json", Content: strings.Join(code, "\n")})
		}
	}
	return files
}

func LoadSourceFiles(root string) ([]SourceFile, error) {
	var files []SourceFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata") { return filepath.SkipDir }
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".json" { return nil }
		data, err := os.ReadFile(path); if err != nil { return err }
		rel, err := filepath.Rel(root, path); if err != nil { return err }
		language := "go"
		if ext == ".json" { language = "json" }
		files = append(files, SourceFile{Path: filepath.ToSlash(rel), Language: language, Content: string(data)})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func Assess(c Catalog, files []SourceFile) Result {
	out := Result{SchemaVersion:"1", EngineVersion:"0.1.0", SourceFileCount:len(files), Counts:map[string]int{}, LLMCalls:0}
	for _, a := range c.ADRs {
		if a.Status != "accepted" { continue }
		for _, r := range a.Requirements {
			rr := RequirementResult{ADR:a.ID, ADRVersion:a.Version, RequirementID:r.ID, Title:r.Title, Severity:r.Severity, Evidence:[]string{}, Findings:[]Finding{}}
			switch r.Verifier.Type {
			case "go-import-boundary": assessImportBoundary(&rr, r.Verifier, files)
			case "json-required-keys": assessJSONRequiredKeys(&rr, r.Verifier, files)
			case "json-number-range": assessJSONNumberRange(&rr, r.Verifier, files)
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
	return out
}

func assessImportBoundary(rr *RequirementResult, v Verifier, files []SourceFile) {
	if len(files) == 0 { rr.Status="INDETERMINATE"; rr.Reason="No source files supplied or discovered; absence of evidence is not compliance."; return }
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
		rr.Status = "NOT_APPLICABLE"
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
		rr.Status = "NOT_APPLICABLE"
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

func pathMatches(path string, prefixes []string) bool {
	path=strings.TrimPrefix(filepath.ToSlash(path),"./")
	for _, p := range prefixes { p=strings.TrimPrefix(filepath.ToSlash(p),"./"); if p!="" && strings.HasPrefix(path,p) { return true } }
	return false
}
func importMatches(p, forbidden string) bool {
	forbidden=strings.TrimSuffix(strings.TrimSpace(forbidden),"/")
	return forbidden!="" && (p==forbidden || strings.HasPrefix(p,forbidden+"/"))
}
func rollup(rs []RequirementResult) string {
	if len(rs)==0 { return "INDETERMINATE" }
	passed:=0
	for _, r := range rs { switch r.Status { case "FAIL": return "FAIL"; case "ERROR","INDETERMINATE","EVIDENCE_MISSING": return "INDETERMINATE"; case "PASS": passed++ } }
	if passed==0 { return "INDETERMINATE" }
	return "PASS"
}
