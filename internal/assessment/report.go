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
	if r.CoverageStatus != "COMPLETE" {
		b.WriteString("> Coverage is incomplete; requirements without applicable evidence are not counted as passing. Missing required evidence is reported as EVIDENCE_MISSING and makes the verdict INDETERMINATE.\n\n")
	}
	fmt.Fprintf(&b, "- **Engine:** %s\n- **Runtime LLM calls:** %d\n- **Source files:** %d\n- **Requirements evaluated:** %d\n", r.EngineVersion, r.LLMCalls, r.SourceFileCount, r.RequirementCount)
	fmt.Fprintf(&b, "- **Corpus coverage:** inventory=%d accepted=%d interpreted=%d bound=%d evidence_complete=%d assessment_complete=%d\n\n",
		r.CorpusCoverage.InventoryADRs, r.CorpusCoverage.AcceptedADRs, r.CorpusCoverage.InterpretedRequirements,
		r.CorpusCoverage.BoundRequirements, r.CorpusCoverage.EvidenceComplete, r.CorpusCoverage.AssessmentComplete)
	if len(r.UnassessedSourceFiles) > 0 {
		b.WriteString("## Unassessed source snippets\n\n")
		b.WriteString("These submitted snippets were retained in the evidence inventory but were not analyzed by a compatible deterministic verifier. Issue assessments do not execute external analyzers, so the result is INDETERMINATE unless a confirmed violation already makes it FAIL.\n\n")
		for _, source := range r.UnassessedSourceFiles {
			fmt.Fprintf(&b, "- %s\n", strings.ReplaceAll(source, "|", "\\|"))
		}
		b.WriteString("\n")
	}
	if len(r.AnalyzerResults) > 0 {
		b.WriteString("## Analyzer executions\n\n| Analyzer | Version | Status | Findings | Input SHA-256 | Configuration SHA-256 | Raw output SHA-256 |\n|---|---|---|---:|---|---|---|\n")
		for _, scan := range r.AnalyzerResults {
			name, version := "unknown", ""
			if len(scan.Analyzers) > 0 {
				name, version = scan.Analyzers[0].Name, scan.Analyzers[0].Version
			}
			fmt.Fprintf(&b, "| %s | %s | **%s** | %d | \x60%s\x60 | \x60%s\x60 | \x60%s\x60 |\n", name, version, scan.Status, len(scan.Findings), scan.InputSHA256, scan.ConfigurationSHA256, scan.RawOutputSHA256)
			for _, problem := range scan.Errors {
				fmt.Fprintf(&b, "  - Analyzer error: %s\n", strings.ReplaceAll(problem, "|", "\\|"))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## Requirement results\n\n| ADR | Requirement | Status | Severity | Details |\n|---|---|---|---|---|\n")
	for _, q := range r.Requirements {
		detail := strings.ReplaceAll(q.Reason, "|", "\\|")
		fmt.Fprintf(&b, "| %s v%d | %s: %s | **%s** | %s | %s |\n", q.ADR, q.ADRVersion, q.RequirementID, q.Title, q.Status, q.Severity, detail)
	}
	if len(r.Requirements) == 0 {
		b.WriteString("| — | No accepted requirements found | INDETERMINATE | — | Catalog contains no assessable requirements. |\n")
	}
	b.WriteString("\n## Findings\n\n")
	count := 0
	for _, q := range r.Requirements {
		for _, f := range q.Findings {
			count++
			location := fmt.Sprintf("%s:%d", f.Path, f.Line)
			if f.Column > 0 {
				location += fmt.Sprintf(":%d", f.Column)
			}
			if f.Analyzer != "" {
				fmt.Fprintf(&b, "- **%s** (%s %s) — \x60%s\x60: %s\n", f.RuleID, f.Analyzer, f.AnalyzerVersion, location, f.Message)
			} else {
				fmt.Fprintf(&b, "- **%s** — \x60%s\x60: %s\n", f.RuleID, location, f.Message)
			}
		}
	}
	if count == 0 {
		b.WriteString("No deterministic violations were identified among checks that produced findings. Review coverage and requirement statuses before treating this as a complete assessment.\n")
	}
	b.WriteString("\n## Evidence and limitations\n\n")
	for _, q := range r.Requirements {
		fmt.Fprintf(&b, "- **%s (%s):** ", q.RequirementID, q.Status)
		if len(q.Evidence) == 0 {
			b.WriteString("No positive evidence recorded. ")
		} else {
			b.WriteString(strings.Join(q.Evidence, " "))
			b.WriteString(" ")
		}
		b.WriteString(q.Reason + "\n")
	}
	b.WriteString("\n> Deterministic local analysis only. Submitted source is parsed as data and never executed. PASS applies only to the declared requirement scope and evidence supplied.\n")
	return b.String()
}

func SARIFReport(r Result) ([]byte, error) {
	rules := map[string]map[string]any{}
	results := []map[string]any{}
	for _, q := range r.Requirements {
		for _, f := range q.Findings {
			rules[f.RuleID] = map[string]any{"id": f.RuleID, "name": q.Title, "shortDescription": map[string]any{"text": q.Title}}
			level := "error"
			if q.Severity == "warning" {
				level = "warning"
			}
			if q.Severity == "info" {
				level = "note"
			}
			properties := map[string]any{
				"adr_id":             q.ADR,
				"adr_version":        q.ADRVersion,
				"requirement_id":     q.RequirementID,
				"requirement_status": q.Status,
				"severity":           q.Severity,
			}
			if f.Analyzer != "" {
				properties["analyzer"] = f.Analyzer
			}
			if f.AnalyzerVersion != "" {
				properties["analyzer_version"] = f.AnalyzerVersion
			}
			if f.Fingerprint != "" {
				properties["fingerprint"] = f.Fingerprint
			}
			results = append(results, map[string]any{
				"ruleId": f.RuleID, "level": level, "message": map[string]any{"text": f.Message},
				"properties": properties,
				"locations": []any{map[string]any{"physicalLocation": map[string]any{
					"artifactLocation": map[string]any{"uri": f.Path},
					"region":           sarifRegion(f),
				}}},
			})
			if f.Fingerprint != "" {
				results[len(results)-1]["partialFingerprints"] = map[string]string{"primaryLocationLineHash": f.Fingerprint}
			}
		}
	}
	ruleList := make([]map[string]any, 0, len(rules))
	ruleIDs := make([]string, 0, len(rules))
	for id := range rules {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs {
		ruleList = append(ruleList, rules[id])
	}
	doc := map[string]any{
		"version": "2.1.0",
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"runs": []any{map[string]any{
			"tool":       map[string]any{"driver": map[string]any{"name": "adrctl", "informationUri": "https://github.com/tyler-beach/pf_2", "rules": ruleList}},
			"results":    results,
			"properties": map[string]any{"adrctl": sarifAssessmentMetadata(r)},
		}},
	}
	return json.MarshalIndent(doc, "", "  ")
}

// sarifAssessmentMetadata preserves assessment state even when the finding
// list is empty. SARIF results are findings, not a complete compliance verdict.
// Only identifiers, statuses, evidence summaries and digests are emitted.
func sarifAssessmentMetadata(r Result) map[string]any {
	statusCounts := make(map[string]int, len(r.Counts)+len(r.Requirements))
	for status, count := range r.Counts {
		statusCounts[status] = count
	}
	if len(statusCounts) == 0 && len(r.Requirements) > 0 {
		for _, requirement := range r.Requirements {
			statusCounts[requirement.Status]++
		}
	}

	requirements := make([]map[string]any, 0, len(r.Requirements))
	for _, requirement := range r.Requirements {
		evidence := append([]string(nil), requirement.Evidence...)
		requirements = append(requirements, map[string]any{
			"adr_id":         requirement.ADR,
			"adr_version":    requirement.ADRVersion,
			"requirement_id": requirement.RequirementID,
			"title":          requirement.Title,
			"status":         requirement.Status,
			"severity":       requirement.Severity,
			"reason":         requirement.Reason,
			"evidence":       evidence,
		})
	}
	sort.Slice(requirements, func(i, j int) bool {
		leftADR, rightADR := fmt.Sprint(requirements[i]["adr_id"]), fmt.Sprint(requirements[j]["adr_id"])
		if leftADR != rightADR {
			return leftADR < rightADR
		}
		return fmt.Sprint(requirements[i]["requirement_id"]) < fmt.Sprint(requirements[j]["requirement_id"])
	})

	unassessed := append([]string(nil), r.UnassessedSourceFiles...)
	sort.Strings(unassessed)

	analyzerRuns := make([]map[string]any, 0, len(r.AnalyzerResults))
	for _, scan := range r.AnalyzerResults {
		analyzers := make([]map[string]any, 0, len(scan.Analyzers))
		for _, analyzerInfo := range scan.Analyzers {
			analyzers = append(analyzers, map[string]any{
				"name":    analyzerInfo.Name,
				"version": analyzerInfo.Version,
			})
		}
		sort.Slice(analyzers, func(i, j int) bool {
			leftName, rightName := fmt.Sprint(analyzers[i]["name"]), fmt.Sprint(analyzers[j]["name"])
			if leftName != rightName {
				return leftName < rightName
			}
			return fmt.Sprint(analyzers[i]["version"]) < fmt.Sprint(analyzers[j]["version"])
		})

		scannedPaths := append([]string(nil), scan.ScannedPaths...)
		sort.Strings(scannedPaths)
		errors := append([]string(nil), scan.Errors...)
		sort.Strings(errors)
		analyzerRuns = append(analyzerRuns, map[string]any{
			"status":               scan.Status,
			"analyzers":            analyzers,
			"finding_count":        len(scan.Findings),
			"input_sha256":         scan.InputSHA256,
			"configuration_sha256": scan.ConfigurationSHA256,
			"binary_sha256":        scan.BinarySHA256,
			"rule_set_sha256":      scan.RuleSetSHA256,
			"raw_output_sha256":    scan.RawOutputSHA256,
			"scanned_paths":        scannedPaths,
			"errors":               errors,
		})
	}
	sort.Slice(analyzerRuns, func(i, j int) bool {
		left, right := analyzerRuns[i], analyzerRuns[j]
		if left["status"] != right["status"] {
			return fmt.Sprint(left["status"]) < fmt.Sprint(right["status"])
		}
		if left["input_sha256"] != right["input_sha256"] {
			return fmt.Sprint(left["input_sha256"]) < fmt.Sprint(right["input_sha256"])
		}
		return fmt.Sprint(left["raw_output_sha256"]) < fmt.Sprint(right["raw_output_sha256"])
	})

	return map[string]any{
		"schema_version":               r.SchemaVersion,
		"engine_version":               r.EngineVersion,
		"verdict":                      r.Verdict,
		"coverage_status":              r.CoverageStatus,
		"evidence_mode":                r.EvidenceMode,
		"requirement_count":            r.RequirementCount,
		"applicable_requirement_count": r.ApplicableRequirementCount,
		"status_counts":                statusCounts,
		"source_file_count":            r.SourceFileCount,
		"source_snapshot_sha256":       r.SourceSnapshotSHA256,
		"runtime_llm_calls":            r.LLMCalls,
		"unassessed_source_files":      unassessed,
		"requirement_statuses":         requirements,
		"analyzer_runs":                analyzerRuns,
	}
}

func sarifRegion(f Finding) map[string]any {
	region := map[string]any{"startLine": max(1, f.Line)}
	if f.Column > 0 {
		region["startColumn"] = f.Column
	}
	return region
}
