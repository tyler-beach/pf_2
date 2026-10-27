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
	SchemaVersion string
	ADRs []ADR
}
type ADR struct {
	ID string
	Title string
	Version int
	Status string
	Requirements []Requirement
}
type Requirement struct {
	ID string
	Title string
	Description string
	Severity string
	Verifier Verifier
}
type Verifier struct {
	Type string
	SourcePathPrefixes []string
	ForbiddenImportPrefixes []string
}
type SourceFile struct {
	Path string
	Language string
	Content string
}
type Finding struct {
	RuleID string
	Path string
	Line int
	Message string
}
type RequirementResult struct {
	ADR string
	ADRVersion int
	RequirementID string
	Title string
	Status string
	Severity string
	Evidence []string
	Findings []Finding
	Reason string
}
type Result struct {
	SchemaVersion string
	EngineVersion string
	Verdict string
	SourceFileCount int
	RequirementCount int
	Counts map[string]int
	LLMCalls int
	Requirements []RequirementResult
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
		}
	}
	return files
}

func LoadGoFiles(root string) ([]SourceFile, error) {
	var files []SourceFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == ".git" || name == "vendor" || name == "node_modules" || name == "testdata") { return filepath.SkipDir }
			return nil
		}
		if filepath.Ext(path) != ".go" { return nil }
		data, err := os.ReadFile(path); if err != nil { return err }
		rel, err := filepath.Rel(root, path); if err != nil { return err }
		files = append(files, SourceFile{Path: filepath.ToSlash(rel), Language: "go", Content: string(data)})
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
			if r.Verifier.Type == "go-import-boundary" { assessImportBoundary(&rr, r.Verifier, files) } else { rr.Status="ERROR"; rr.Reason="Unsupported verifier type: "+r.Verifier.Type }
			out.Counts[rr.Status]++
			out.Requirements = append(out.Requirements, rr)
		}
	}
	out.RequirementCount = len(out.Requirements)
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
	for _, r := range rs { switch r.Status { case "FAIL": return "FAIL"; case "ERROR","INDETERMINATE": return "INDETERMINATE"; case "PASS": passed++ } }
	if passed==0 { return "INDETERMINATE" }
	return "PASS"
}
