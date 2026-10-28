package prscan

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildSecuritySARIFShape(t *testing.T) {
	conv := ConvertAssessment(sampleResult(), ConvertOptions{
		MaxInline: 10,
		DocsBase:  "https://example.test/#/docs/pr-scan",
		PRURL:     "https://github.com/acme/widgets/pull/9",
		HeadSHA:   "deadbeefcafebabe",
	})
	doc := conv.SecuritySARIF
	if doc.Version != "2.1.0" || len(doc.Runs) != 1 {
		t.Fatalf("bad envelope: %+v", doc)
	}
	run := doc.Runs[0]
	if run.Tool.Driver.Name != "Pathfinder" {
		t.Fatalf("driver: %+v", run.Tool.Driver)
	}
	if len(run.Results) != 2 {
		t.Fatalf("want 2 results (no N/A), got %d", len(run.Results))
	}
	for _, r := range run.Results {
		if !strings.HasPrefix(r.RuleID, "pathfinder/") {
			t.Fatalf("unstable ruleId %q", r.RuleID)
		}
		if r.PartialFingerprints["primaryLocationLineHash"] == "" {
			t.Fatal("missing partialFingerprint")
		}
		if len(r.Locations) == 0 || r.Locations[0].PhysicalLocation.ArtifactLocation.URI == "" {
			t.Fatal("missing location")
		}
		if !strings.Contains(r.Message.Text, "Why:") || !strings.Contains(r.Message.Text, "Next:") {
			t.Fatalf("message not shared formatter:\n%s", r.Message.Text)
		}
	}
	if len(run.Tool.Driver.Rules) == 0 {
		t.Fatal("missing rules")
	}
	for _, rule := range run.Tool.Driver.Rules {
		if rule.HelpURI == "" || rule.Help == nil {
			t.Fatalf("rule missing help: %+v", rule)
		}
		if rule.Properties["adr_id"] == nil {
			t.Fatalf("rule missing adr_id: %+v", rule)
		}
	}
	if len(run.VersionControlProvenance) != 1 || run.VersionControlProvenance[0].RevisionID != "deadbeefcafebabe" {
		t.Fatalf("vcs provenance: %+v", run.VersionControlProvenance)
	}
	if run.VersionControlProvenance[0].RepositoryURI != "https://github.com/acme/widgets" {
		t.Fatalf("repo uri: %q", run.VersionControlProvenance[0].RepositoryURI)
	}

	raw, err := MarshalSecuritySARIF(doc)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["version"] != "2.1.0" {
		t.Fatalf("json version: %v", decoded["version"])
	}
}

func TestSecuritySARIFOmitsNA(t *testing.T) {
	raw, err := MarshalSecuritySARIF(ConvertAssessment(sampleResult(), ConvertOptions{}).SecuritySARIF)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "should never post") || strings.Contains(string(raw), "REQ-NA") {
		t.Fatal("N/A leaked into security SARIF")
	}
}
