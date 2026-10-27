package assessment

import (
	"encoding/json"
	"fmt"
	"strings"
)

func MarkdownReport(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# ADR Compliance Assessment: %s\n\n", r.Verdict)
	fmt.Fprintf(&b, "- **Engine:** %s\n- **Runtime LLM calls:** %d\n- **Source files:** %d\n- **Requirements evaluated:** %d\n\n", r.EngineVersion, r.LLMCalls, r.SourceFileCount, r.RequirementCount)
	b.WriteString("## Requirement results\n\n| ADR | Requirement | Status | Severity | Details |\n|---|---|---|---|---|\n")
	for _, q := range r.Requirements {
		detail := strings.ReplaceAll(q.Reason, "|", "\\|")
		fmt.Fprintf(&b, "| %s v%d | %s: %s | **%s** | %s | %s |\n", q.ADR, q.ADRVersion, q.RequirementID, q.Title, q.Status, q.Severity, detail)
	}
	if len(r.Requirements)==0 { b.WriteString("| — | No accepted requirements found | INDETERMINATE | — | Catalog contains no assessable requirements. |\n") }
	b.WriteString("\n## Findings\n\n")
	count:=0
	for _, q := range r.Requirements {
		for _, f := range q.Findings { count++; fmt.Fprintf(&b, "- **%s** — `%s:%d`: %s\n", f.RuleID, f.Path, f.Line, f.Message) }
	}
	if count==0 { b.WriteString("No deterministic violations were identified. This is not a pass if any required check was indeterminate or errored.\n") }
	b.WriteString("\n## Evidence and limitations\n\n")
	for _, q := range r.Requirements {
		fmt.Fprintf(&b, "- **%s (%s):** ", q.RequirementID, q.Status)
		if len(q.Evidence)==0 { b.WriteString("No positive evidence recorded. ") } else { b.WriteString(strings.Join(q.Evidence," ")); b.WriteString(" ") }
		b.WriteString(q.Reason+"\n")
	}
	b.WriteString("\n> Deterministic local analysis only. Submitted source is parsed as data and never executed. PASS applies only to the declared requirement scope and evidence supplied.\n")
	return b.String()
}

func SARIFReport(r Result) ([]byte, error) {
	rules:=map[string]map[string]any{}
	var results []map[string]any
	for _, q := range r.Requirements {
		for _, f := range q.Findings {
			rules[f.RuleID]=map[string]any{"id":f.RuleID,"name":q.Title,"shortDescription":map[string]any{"text":q.Title}}
			level:="error"; if q.Severity=="warning" { level="warning" }; if q.Severity=="info" { level="note" }
			results=append(results,map[string]any{
				"ruleId":f.RuleID,"level":level,"message":map[string]any{"text":f.Message},
				"locations":[]any{map[string]any{"physicalLocation":map[string]any{
					"artifactLocation":map[string]any{"uri":f.Path},
					"region":map[string]any{"startLine":max(1,f.Line)},
				}}},
			})
		}
	}
	ruleList:=make([]map[string]any,0,len(rules))
	for _, v:=range rules { ruleList=append(ruleList,v) }
	doc:=map[string]any{
		"version":"2.1.0",
		"$schema":"https://json.schemastore.org/sarif-2.1.0.json",
		"runs":[]any{map[string]any{
			"tool":map[string]any{"driver":map[string]any{"name":"adrctl","informationUri":"https://github.com/tyler-beach/pf_2","rules":ruleList}},
			"results":results,
		}},
	}
	return json.MarshalIndent(doc,"","  ")
}
