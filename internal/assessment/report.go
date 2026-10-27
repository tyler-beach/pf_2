package assessment

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func MarkdownReport(r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# ADR Compliance Assessment: %s\n\n", r.Verdict)
	fmt.Fprintf(&b, "- **Coverage:** %s (%d/%d requirements applicable)\n- **Evidence scope:** %s\n", r.CoverageStatus, r.ApplicableRequirementCount, r.RequirementCount, r.EvidenceMode)
	if r.CoverageStatus != "COMPLETE" { b.WriteString("> Coverage is incomplete; requirements without applicable evidence are not counted as passing. Missing required evidence is reported as EVIDENCE_MISSING and makes the verdict INDETERMINATE.\n\n") }
	fmt.Fprintf(&b, "- **Engine:** %s\n- **Runtime LLM calls:** %d\n- **Source files:** %d\n- **Requirements evaluated:** %d\n\n", r.EngineVersion, r.LLMCalls, r.SourceFileCount, r.RequirementCount)
	if len(r.AnalyzerResults) > 0 {
		b.WriteString("## Analyzer executions\n\n| Analyzer | Version | Status | Findings | Input SHA-256 | Configuration SHA-256 | Raw output SHA-256 |\n|---|---|---|---:|---|---|---|\n")
		for _, scan := range r.AnalyzerResults {
			name, version := "unknown", ""
			if len(scan.Analyzers) > 0 { name, version = scan.Analyzers[0].Name, scan.Analyzers[0].Version }
			fmt.Fprintf(&b, "| %s | %s | **%s** | %d | `%s` | `%s` | `%s` |\n", name, version, scan.Status, len(scan.Findings), scan.InputSHA256, scan.ConfigurationSHA256, scan.RawOutputSHA256)
			for _, problem := range scan.Errors { fmt.Fprintf(&b, "  - Analyzer error: %s\n", strings.ReplaceAll(problem, "|", "\\|")) }
		}
		b.WriteString("\n")
	}
	b.WriteString("## Requirement results\n\n| ADR | Requirement | Status | Severity | Details |\n|---|---|---|---|---|\n")
	for _, q := range r.Requirements {
		detail := strings.ReplaceAll(q.Reason, "|", "\\|")
		fmt.Fprintf(&b, "| %s v%d | %s: %s | **%s** | %s | %s |\n", q.ADR, q.ADRVersion, q.RequirementID, q.Title, q.Status, q.Severity, detail)
	}
	if len(r.Requirements)==0 { b.WriteString("| — | No accepted requirements found | INDETERMINATE | — | Catalog contains no assessable requirements. |\n") }
	b.WriteString("\n## Findings\n\n")
	count:=0
	for _, q := range r.Requirements {
		for _, f := range q.Findings {
			count++
			location := fmt.Sprintf("%s:%d", f.Path, f.Line)
			if f.Column > 0 { location += fmt.Sprintf(":%d", f.Column) }
			if f.Analyzer != "" {
				fmt.Fprintf(&b, "- **%s** (%s %s) — `%s`: %s\n", f.RuleID, f.Analyzer, f.AnalyzerVersion, location, f.Message)
			} else {
				fmt.Fprintf(&b, "- **%s** — `%s`: %s\n", f.RuleID, location, f.Message)
			}
		}
	}
	if count==0 { b.WriteString("No deterministic violations were identified among checks that produced findings. Review coverage and requirement statuses before treating this as a complete assessment.\n") }
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
	results := []map[string]any{}
	for _, q := range r.Requirements {
		for _, f := range q.Findings {
			rules[f.RuleID]=map[string]any{"id":f.RuleID,"name":q.Title,"shortDescription":map[string]any{"text":q.Title}}
			level:="error"; if q.Severity=="warning" { level="warning" }; if q.Severity=="info" { level="note" }
			results=append(results,map[string]any{
				"ruleId":f.RuleID,"level":level,"message":map[string]any{"text":f.Message},
				"locations":[]any{map[string]any{"physicalLocation":map[string]any{
					"artifactLocation":map[string]any{"uri":f.Path},
					"region":sarifRegion(f),
				}}},
			})
			if f.Fingerprint != "" {
				results[len(results)-1]["partialFingerprints"] = map[string]string{"primaryLocationLineHash": f.Fingerprint}
			}
		}
	}
	ruleList:=make([]map[string]any,0,len(rules))
	ruleIDs := make([]string, 0, len(rules))
	for id := range rules { ruleIDs = append(ruleIDs, id) }
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs { ruleList = append(ruleList, rules[id]) }
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

func sarifRegion(f Finding) map[string]any {
	region := map[string]any{"startLine": max(1, f.Line)}
	if f.Column > 0 { region["startColumn"] = f.Column }
	return region
}
