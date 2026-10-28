package prscan

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SecuritySARIF is a GitHub code-scanning–oriented SARIF 2.1.0 document built
// from shared Pathfinder findings (deviations only).
type SecuritySARIF struct {
	Version string        `json:"version"`
	Schema  string        `json:"$schema"`
	Runs    []SecurityRun `json:"runs"`
}

// SecurityRun is one SARIF run.
type SecurityRun struct {
	Tool                     SecurityTool       `json:"tool"`
	Results                  []SecurityResult   `json:"results"`
	AutomationDetails        *AutomationDetails `json:"automationDetails,omitempty"`
	VersionControlProvenance []VCSProvenance    `json:"versionControlProvenance,omitempty"`
}

// SecurityTool wraps the driver.
type SecurityTool struct {
	Driver SecurityDriver `json:"driver"`
}

// SecurityDriver identifies Pathfinder as a SAST-like tool.
type SecurityDriver struct {
	Name           string         `json:"name"`
	InformationURI string         `json:"informationUri,omitempty"`
	Rules          []SecurityRule `json:"rules"`
}

// SecurityRule is a reportingDescriptor for the Security tab.
type SecurityRule struct {
	ID               string             `json:"id"`
	Name             string             `json:"name,omitempty"`
	ShortDescription *MessageText       `json:"shortDescription,omitempty"`
	FullDescription  *MessageText       `json:"fullDescription,omitempty"`
	HelpURI          string             `json:"helpUri,omitempty"`
	Help             *MessageText       `json:"help,omitempty"`
	DefaultConfig    *RuleDefaultConfig `json:"defaultConfiguration,omitempty"`
	Properties       map[string]any     `json:"properties,omitempty"`
}

// RuleDefaultConfig sets default level.
type RuleDefaultConfig struct {
	Level string `json:"level,omitempty"`
}

// MessageText is SARIF message/help text.
type MessageText struct {
	Text     string `json:"text,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

// SecurityResult is one code-scanning alert.
type SecurityResult struct {
	RuleID              string             `json:"ruleId"`
	Level               string             `json:"level,omitempty"`
	Message             MessageText        `json:"message"`
	Locations           []SecurityLocation `json:"locations"`
	PartialFingerprints map[string]string  `json:"partialFingerprints,omitempty"`
	Properties          map[string]any     `json:"properties,omitempty"`
}

// SecurityLocation points at a file region.
type SecurityLocation struct {
	PhysicalLocation SecurityPhysical `json:"physicalLocation"`
}

// SecurityPhysical is artifact + region.
type SecurityPhysical struct {
	ArtifactLocation SecurityArtifact `json:"artifactLocation"`
	Region           *SecurityRegion  `json:"region,omitempty"`
}

// SecurityArtifact is a URI relative to the repo root.
type SecurityArtifact struct {
	URI string `json:"uri"`
}

// SecurityRegion is 1-indexed.
type SecurityRegion struct {
	StartLine   int `json:"startLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
}

// AutomationDetails categories the upload in the Security UI.
type AutomationDetails struct {
	ID string `json:"id,omitempty"`
}

// VCSProvenance associates results with the scanned commit (cross-repo).
type VCSProvenance struct {
	RepositoryURI string `json:"repositoryUri,omitempty"`
	RevisionID    string `json:"revisionId,omitempty"`
}

// BuildSecuritySARIF maps shared findings into GitHub-friendly SARIF.
// Messages use the same FormatComment shape as reviewdog so Security and PR
// comments stay complementary, not contradictory.
func BuildSecuritySARIF(findings []Finding, opt ConvertOptions) SecuritySARIF {
	if opt.DocsBase == "" {
		opt.DocsBase = DefaultDocsBase
	}
	rules := map[string]SecurityRule{}
	results := make([]SecurityResult, 0, len(findings))

	for _, f := range findings {
		if _, ok := rules[f.RuleID]; !ok {
			helpMD := fmt.Sprintf(
				"**ADR:** %s  \n**Requirement:** `%s`  \n\n%s\n\n**Remediation:** Fix the deviation, then re-run Pathfinder PR scan. Docs: %s",
				f.ADRID, f.RequirementID, firstNonEmpty(f.Reason, f.Why), f.HelpURI,
			)
			rules[f.RuleID] = SecurityRule{
				ID:               f.RuleID,
				Name:             firstNonEmpty(f.Title, f.RequirementID),
				ShortDescription: &MessageText{Text: firstNonEmpty(f.Title, f.RequirementID)},
				FullDescription:  &MessageText{Text: firstNonEmpty(f.Reason, f.Why, f.Title)},
				HelpURI:          f.HelpURI,
				Help:             &MessageText{Text: compactOneLine(firstNonEmpty(f.Reason, f.Why)), Markdown: helpMD},
				DefaultConfig:    &RuleDefaultConfig{Level: sarifLevel(f.Severity)},
				Properties: map[string]any{
					"precision":         "high",
					"security-severity": securitySeverityScore(f.Severity),
					"tags":              []string{"pathfinder", "adr", "security"},
					"adr_id":            f.ADRID,
					"requirement_id":    f.RequirementID,
				},
			}
		}

		region := (*SecurityRegion)(nil)
		if f.Line > 0 {
			region = &SecurityRegion{StartLine: f.Line}
			if f.Column > 0 {
				region.StartColumn = f.Column
			}
		}
		// Security tab title prefers first sentence; keep shared body for detail.
		msg := MessageText{
			Text:     f.Message,
			Markdown: "```\n" + f.Message + "\n```",
		}
		results = append(results, SecurityResult{
			RuleID:  f.RuleID,
			Level:   sarifLevel(f.Severity),
			Message: msg,
			Locations: []SecurityLocation{{
				PhysicalLocation: SecurityPhysical{
					ArtifactLocation: SecurityArtifact{URI: f.Path},
					Region:           region,
				},
			}},
			PartialFingerprints: map[string]string{
				"primaryLocationLineHash": f.Fingerprint,
			},
			Properties: map[string]any{
				"adr_id":         f.ADRID,
				"requirement_id": f.RequirementID,
				"analyzer":       f.Analyzer,
			},
		})
	}

	ruleIDs := make([]string, 0, len(rules))
	for id := range rules {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	ruleList := make([]SecurityRule, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		ruleList = append(ruleList, rules[id])
	}

	run := SecurityRun{
		Tool: SecurityTool{Driver: SecurityDriver{
			Name:           "Pathfinder",
			InformationURI: firstNonEmpty(opt.SourceURL, "https://github.com/tyler-beach/pf_2"),
			Rules:          ruleList,
		}},
		Results: results,
		AutomationDetails: &AutomationDetails{
			ID: "pathfinder/pr-scan",
		},
	}
	if opt.HeadSHA != "" && opt.PRURL != "" {
		// Best-effort repo URI from PR URL (https://github.com/o/r/pull/N → https://github.com/o/r).
		repoURI := trimPullSuffix(opt.PRURL)
		run.VersionControlProvenance = []VCSProvenance{{
			RepositoryURI: repoURI,
			RevisionID:    opt.HeadSHA,
		}}
	}

	return SecuritySARIF{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs:    []SecurityRun{run},
	}
}

// MarshalSecuritySARIF returns pretty JSON.
func MarshalSecuritySARIF(doc SecuritySARIF) ([]byte, error) {
	return json.MarshalIndent(doc, "", "  ")
}

func sarifLevel(sev string) string {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "ERROR":
		return "error"
	case "WARNING":
		return "warning"
	case "INFO":
		return "note"
	default:
		return "error"
	}
}

func securitySeverityScore(sev string) string {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "ERROR":
		return "8.0"
	case "WARNING":
		return "5.0"
	default:
		return "2.0"
	}
}

func trimPullSuffix(prURL string) string {
	prURL = strings.TrimSpace(prURL)
	if i := strings.Index(strings.ToLower(prURL), "/pull/"); i > 0 {
		return prURL[:i]
	}
	return prURL
}
