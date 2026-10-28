package prscan

import (
	"strings"
	"testing"
)

func TestParseIssueBody(t *testing.T) {
	body := `
### Pull request URL

https://github.com/acme/widgets/pull/99

### Feedback modes (reviewdog)

- [x] ` + "`github-pr-review`" + ` — inline line comments on deviations
- [ ] ` + "`github-pr-check`" + ` — Checks API annotations
- [x] ` + "`github-check`" + ` — commit Check Run
- [ ] ` + "`github-pr-annotations`" + ` — Actions job annotations
- [x] ` + "`local`" + ` — upload rdjson artifact only (no remote post)

### Diff filter mode

added

### Requirement catalog

Corpus pack (testdata/corpus/requirements.json)

### Fail level

any
`
	got, err := ParseIssueBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if got.PR.Owner != "acme" || got.PR.Repo != "widgets" || got.PR.Number != 99 {
		t.Fatalf("PR: %+v", got.PR)
	}
	wantReporters := []string{"github-pr-review", "github-check", "local"}
	if strings.Join(got.Reporters, ",") != strings.Join(wantReporters, ",") {
		t.Fatalf("reporters: %v", got.Reporters)
	}
	if got.FilterMode != "added" || got.Catalog != "corpus" || got.FailLevel != "any" {
		t.Fatalf("opts: %+v", got)
	}
}

func TestParseIssueBodyDefaultFailLevelAny(t *testing.T) {
	body := `
### Pull request URL
https://github.com/acme/widgets/pull/3

### Feedback modes (reviewdog)
- [x] ` + "`github-pr-review`" + `

### Diff filter mode
added

### Requirement catalog
Corpus pack (testdata/corpus/requirements.json)
`
	got, err := ParseIssueBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if got.FailLevel != "any" {
		t.Fatalf("default FailLevel: %q", got.FailLevel)
	}
}

func TestParseIssueBodyRequiresReporterOrSARIF(t *testing.T) {
	body := `
### Pull request URL
https://github.com/acme/widgets/pull/1

### Feedback modes (reviewdog)
- [ ] ` + "`github-pr-review`" + `
`
	if _, err := ParseIssueBody(body); err == nil {
		t.Fatal("expected error when no reporters and no SARIF")
	}
}

func TestParseIssueBodySARIFOnly(t *testing.T) {
	body := `
### Pull request URL
https://github.com/acme/widgets/pull/2

### Feedback modes (reviewdog)
- [ ] ` + "`github-pr-review`" + `

### GitHub Security (code scanning)
- [x] Upload SARIF to GitHub code scanning (Security tab)

### Diff filter mode
added

### Requirement catalog
Corpus pack (testdata/corpus/requirements.json)

### Fail level
none
`
	got, err := ParseIssueBody(body)
	if err != nil {
		t.Fatal(err)
	}
	if !got.UploadSARIF {
		t.Fatal("expected UploadSARIF")
	}
	if len(got.Reporters) != 0 {
		t.Fatalf("reporters: %v", got.Reporters)
	}
}
