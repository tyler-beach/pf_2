package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/adringest"
)

func main() {
	if len(os.Args)<2 { usage(); os.Exit(2) }
	var err error
	switch os.Args[1] {
	case "validate": err=runValidate(os.Args[2:])
	case "assess": err=runAssess(os.Args[2:],false)
	case "assess-repo": err=runAssess(os.Args[2:],true)
	case "ingest-adrs": err=runIngestADRs(os.Args[2:])
	case "version": fmt.Println("adrctl 0.1.0")
	default: usage(); os.Exit(2)
	}
	if err!=nil { fmt.Fprintln(os.Stderr,"adrctl:",err); os.Exit(1) }
}
func usage() { fmt.Fprintln(os.Stderr,"usage: adrctl <validate|assess|assess-repo|ingest-adrs|version> [flags]") }
func runValidate(args []string) error {
	fs:=flag.NewFlagSet("validate",flag.ContinueOnError)
	path:=fs.String("requirements","adr/requirements.json","requirements catalog JSON")
	if err:=fs.Parse(args); err!=nil { return err }
	c,err:=assessment.LoadCatalog(*path); if err!=nil { return err }
	n:=0; for _,a:=range c.ADRs { n+=len(a.Requirements) }
	fmt.Printf("Catalog valid: %d ADR(s), %d requirement(s).\n",len(c.ADRs),n)
	return nil
}
func runAssess(args []string, repoMode bool) error {
	fs:=flag.NewFlagSet("assess",flag.ContinueOnError)
	input:=fs.String("input","","Markdown issue body")
	root:=fs.String("root",".","repository root")
	catalogPath:=fs.String("requirements","adr/requirements.json","requirements catalog JSON")
	reportPath:=fs.String("report","report.md","Markdown report output")
	jsonPath:=fs.String("json-out","assessment.json","JSON evidence output")
	sarifPath:=fs.String("sarif-out","results.sarif","SARIF output")
	if err:=fs.Parse(args); err!=nil { return err }
	c,err:=assessment.LoadCatalog(*catalogPath); if err!=nil { return err }
	var files []assessment.SourceFile
	if repoMode { files,err=assessment.LoadSourceFiles(*root) } else {
		if *input=="" { return fmt.Errorf("--input is required for issue assessment") }
		var data []byte; data,err=os.ReadFile(*input); if err==nil { files=assessment.ParseIssueMarkdown(string(data)) }
	}
	if err!=nil { return err }
	result:=assessment.Assess(c,files)
	if err:=writeFile(*reportPath,[]byte(assessment.MarkdownReport(result))); err!=nil { return err }
	j,err:=json.MarshalIndent(result,"","  "); if err!=nil { return err }
	if err:=writeFile(*jsonPath,j); err!=nil { return err }
	s,err:=assessment.SARIFReport(result); if err!=nil { return err }
	if err:=writeFile(*sarifPath,s); err!=nil { return err }
	fmt.Printf("Verdict: %s; requirements: %d; source files: %d; LLM calls: %d\n",result.Verdict,result.RequirementCount,result.SourceFileCount,result.LLMCalls)
	fmt.Print(assessment.MarkdownReport(result))
	return nil
}
func writeFile(path string,data []byte) error {
	dir:=filepath.Dir(path)
	if dir!="." { if err:=os.MkdirAll(dir,0755); err!=nil { return err } }
	if err:=os.WriteFile(path,data,0644); err!=nil { return fmt.Errorf("write %s: %w",path,err) }
	return nil
}


// ingestionIndex is a reproducible snapshot of source ADR documents. It is
// deliberately separate from requirements.json: parsing is not approval.
type ingestionIndex struct {
	SchemaVersion string               `json:"schema_version"`
	SourceRoot    string               `json:"source_root"`
	Documents     []adringest.Document `json:"documents"`
}

func runIngestADRs(args []string) error {
	fs := flag.NewFlagSet("ingest-adrs", flag.ContinueOnError)
	root := fs.String("root", "adrs", "directory containing ADR Markdown files")
	output := fs.String("json-out", "adr/ingested-adrs.json", "JSON ingestion index output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	docs, err := adringest.ParseDir(*root)
	if err != nil {
		return err
	}
	seen := make(map[string]string, len(docs))
	for _, doc := range docs {
		if previous, ok := seen[doc.ID]; ok {
			return fmt.Errorf("duplicate ADR ID %q in %s and %s", doc.ID, previous, doc.SourcePath)
		}
		seen[doc.ID] = doc.SourcePath
	}
	index := ingestionIndex{
		SchemaVersion: "1",
		SourceRoot:    filepath.ToSlash(filepath.Clean(*root)),
		Documents:     docs,
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ADR ingestion index: %w", err)
	}
	if err := writeFile(*output, data); err != nil {
		return err
	}
	fmt.Printf("Ingested %d ADR document(s) into %s. No obligations were auto-approved.\n", len(docs), *output)
	return nil
}
