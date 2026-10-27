package assessment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarkdownAndSARIFReports(t *testing.T) {
	files:=[]SourceFile{{Path:"internal/domain/customer.go",Language:"go",Content:"package domain\nimport \"net/http\"\n"}}
	result:=Assess(fixtureCatalog(),files)
	md:=MarkdownReport(result)
	if !strings.Contains(md,"ADR Compliance Assessment: FAIL") || !strings.Contains(md,"REQ-001-02") { t.Fatalf("unexpected Markdown report: %s",md) }
	data,err:=SARIFReport(result); if err!=nil { t.Fatal(err) }
	var parsed map[string]any
	if err:=json.Unmarshal(data,&parsed); err!=nil { t.Fatalf("invalid SARIF JSON: %v",err) }
	if parsed["version"]!="2.1.0" { t.Fatalf("expected SARIF 2.1.0, got %v",parsed["version"]) }
}
