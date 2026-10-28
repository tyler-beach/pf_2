package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tyler-beach/pf_2/internal/adringest"
	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

func reviewCLISetup(t *testing.T) (string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	adrRoot := filepath.Join(root, "adrs")
	if err := os.MkdirAll(adrRoot, 0700); err != nil {
		t.Fatal(err)
	}
	source := "# ADR-001: Preserve domain boundaries\n\n## Status\n\nAccepted\n\n## Decision\n\nDomain code must not import internal infrastructure packages.\n"
	if err := os.WriteFile(filepath.Join(adrRoot, "ADR-001.md"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := adringest.Parse("ADR-001.md", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	rawIngestion := []byte(`{"schema_version":"1","source_root":"adrs","documents":[]}`)
	index, err := obligations.BuildIndex([]adringest.Document{doc}, adrRoot, "adrs", rawIngestion)
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "candidates.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	catalog, err := assessment.LoadCatalog(filepath.Join("..", "..", "adr", "requirements.json"))
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(root, "requirements.json")
	catalogBytes, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalogPath, catalogBytes, 0600); err != nil {
		t.Fatal(err)
	}
	return root, indexPath, catalogPath, filepath.Join(root, "reviews.json")
}

func reviewCLIProposal(t *testing.T, indexPath string) string {
	t.Helper()
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index obligations.Index
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Candidates) == 0 {
		t.Fatal("fixture did not produce a candidate")
	}
	proposal := map[string]any{
		"candidate_id":     index.Candidates[0].ID,
		"decision":         "approved",
		"requirement_id":   "REQ-001-01",
		"pattern_id":       "architecture.dependency_or_boundary",
		"enforcement_mode": "blocking",
		"reviewer":         "security-reviewer@example.com",
		"reviewed_at":      "2026-10-27T12:00:00Z",
		"review_reference": "https://github.com/tyler-beach/pf_2/issues/20",
		"rationale":        "Bound to the existing reviewed Go import-boundary requirement.",
	}
	path := filepath.Join(filepath.Dir(indexPath), "proposal.json")
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReviewCLIInitializesRecordsAndValidatesSourceBoundDecision(t *testing.T) {
	_, indexPath, catalogPath, registryPath := reviewCLISetup(t)
	if err := runInitReviewRegistry([]string{"--output", registryPath}); err != nil {
		t.Fatalf("init: %v", err)
	}
	proposalPath := reviewCLIProposal(t, indexPath)
	coveragePath := filepath.Join(filepath.Dir(registryPath), "coverage.json")
	if err := runRecordObligationReview([]string{
		"--index", indexPath, "--requirements", catalogPath, "--registry", registryPath,
		"--proposal", proposalPath, "--coverage-out", coveragePath,
	}); err != nil {
		t.Fatalf("record review: %v", err)
	}
	registry, err := obligations.LoadRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Records) != 1 || registry.Records[0].Decision != obligations.DecisionApproved ||
		registry.Records[0].RequirementID != "REQ-001-01" {
		t.Fatalf("review decision was not persisted: %+v", registry)
	}
	coverageBytes, err := os.ReadFile(coveragePath)
	if err != nil {
		t.Fatal(err)
	}
	var coverage obligations.BindingCoverage
	if err := json.Unmarshal(coverageBytes, &coverage); err != nil {
		t.Fatal(err)
	}
	if coverage.BlockingBindingCount != 1 || coverage.ApprovedCount != 1 || coverage.PendingReviewCount != 0 {
		t.Fatalf("unexpected coverage: %+v", coverage)
	}
	if err := runValidateObligationReviews([]string{
		"--index", indexPath, "--requirements", catalogPath, "--registry", registryPath, "--strict",
	}); err != nil {
		t.Fatalf("strict review validation: %v", err)
	}
}

func TestReviewCLIRejectsCrossADRBindingWithoutMutatingRegistry(t *testing.T) {
	_, indexPath, catalogPath, registryPath := reviewCLISetup(t)
	if err := runInitReviewRegistry([]string{"--output", registryPath}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	proposalPath := reviewCLIProposal(t, indexPath)
	var proposal map[string]any
	data, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &proposal); err != nil {
		t.Fatal(err)
	}
	proposal["requirement_id"] = "REQ-003-01"
	data, err = json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proposalPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	err = runRecordObligationReview([]string{
		"--index", indexPath, "--requirements", catalogPath, "--registry", registryPath,
		"--proposal", proposalPath,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be bound to requirement") {
		t.Fatalf("expected cross-ADR binding rejection, got %v", err)
	}
	after, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("invalid proposal mutated registry")
	}
}

func TestReviewCLIInitNeverOverwritesExistingAuditLog(t *testing.T) {
	_, _, _, registryPath := reviewCLISetup(t)
	if err := runInitReviewRegistry([]string{"--output", registryPath}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	err = runInitReviewRegistry([]string{"--output", registryPath})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected overwrite guard, got %v", err)
	}
	after, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("init overwrote existing review audit history")
	}
}

func TestReviewCLIValidateReportsStaleBinding(t *testing.T) {
	_, indexPath, catalogPath, registryPath := reviewCLISetup(t)
	if err := runInitReviewRegistry([]string{"--output", registryPath}); err != nil {
		t.Fatal(err)
	}
	proposalPath := reviewCLIProposal(t, indexPath)
	if err := runRecordObligationReview([]string{
		"--index", indexPath, "--requirements", catalogPath, "--registry", registryPath, "--proposal", proposalPath,
	}); err != nil {
		t.Fatal(err)
	}
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index obligations.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		t.Fatal(err)
	}
	index.Candidates[0].SourceSHA256 = strings.Repeat("c", 64)
	index.SourceIndexSHA256 = strings.Repeat("d", 64)
	indexBytes, err = json.MarshalIndent(index, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, indexBytes, 0600); err != nil {
		t.Fatal(err)
	}
	coveragePath := filepath.Join(filepath.Dir(registryPath), "stale-coverage.json")
	err = runValidateObligationReviews([]string{
		"--index", indexPath, "--requirements", catalogPath, "--registry", registryPath,
		"--strict", "--json-out", coveragePath,
	})
	if err == nil || !strings.Contains(err.Error(), "stale candidate decision") {
		t.Fatalf("strict validation should reject stale bindings, got %v", err)
	}
	data, err := os.ReadFile(coveragePath)
	if err != nil {
		t.Fatalf("coverage should still be written for diagnosis: %v", err)
	}
	var coverage obligations.BindingCoverage
	if err := json.Unmarshal(data, &coverage); err != nil {
		t.Fatal(err)
	}
	if coverage.StaleCount != 1 || coverage.BlockingBindingCount != 0 {
		t.Fatalf("stale binding was treated as active: %+v", coverage)
	}
}

func TestReviewCLIRejectsMalformedProposalJSONWithoutMutation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{
			name: "invalid trailing bytes",
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte("\\nnot-json")...)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unknown field",
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var object map[string]any
				if err := json.Unmarshal(data, &object); err != nil {
					t.Fatal(err)
				}
				object["unexpected_field"] = "not allowed"
				data, err = json.Marshal(object)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, indexPath, catalogPath, registryPath := reviewCLISetup(t)
			if err := runInitReviewRegistry([]string{"--output", registryPath}); err != nil {
				t.Fatal(err)
			}
			proposalPath := reviewCLIProposal(t, indexPath)
			tc.mutate(t, proposalPath)
			before, err := os.ReadFile(registryPath)
			if err != nil {
				t.Fatal(err)
			}
			err = runRecordObligationReview([]string{
				"--index", indexPath, "--requirements", catalogPath, "--registry", registryPath,
				"--proposal", proposalPath,
			})
			if err == nil {
				t.Fatal("malformed or unrecognized proposal JSON unexpectedly succeeded")
			}
			after, err := os.ReadFile(registryPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("invalid proposal mutated registry")
			}
		})
	}
}
