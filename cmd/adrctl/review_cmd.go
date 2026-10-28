package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyler-beach/pf_2/internal/assessment"
	"github.com/tyler-beach/pf_2/internal/obligations"
)

func runInitReviewRegistry(args []string) error {
	fs := flag.NewFlagSet("review-init", flag.ContinueOnError)
	output := fs.String("output", "adr/obligation-reviews.json", "new review registry JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("review-init accepts no positional arguments")
	}
	absolute, err := filepath.Abs(*output)
	if err != nil {
		return fmt.Errorf("resolve registry output: %w", err)
	}
	if _, err := os.Lstat(absolute); err == nil {
		return fmt.Errorf("review registry %s already exists; initialization never overwrites review history", absolute)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect registry output %s: %w", absolute, err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0755); err != nil {
		return fmt.Errorf("create registry directory: %w", err)
	}
	data, err := json.MarshalIndent(obligations.NewRegistry(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode empty review registry: %w", err)
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("review registry %s already exists; initialization never overwrites review history", absolute)
		}
		return fmt.Errorf("create review registry: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(absolute)
		}
	}()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write review registry: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync review registry: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close review registry: %w", err)
	}
	ok = true
	fmt.Printf("Initialized empty review registry at %s. No candidate was approved.\n", absolute)
	return nil
}

func runRecordObligationReview(args []string) error {
	fs := flag.NewFlagSet("record-obligation-review", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "review-only candidate index JSON")
	catalogPath := fs.String("requirements", "adr/requirements.json", "approved requirements catalog JSON")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "existing append-only review registry")
	proposalPath := fs.String("proposal", "", "JSON review proposal")
	coveragePath := fs.String("coverage-out", "", "optional binding coverage JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("record-obligation-review accepts no positional arguments")
	}
	if strings.TrimSpace(*proposalPath) == "" {
		return fmt.Errorf("--proposal is required")
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil {
		return err
	}
	proposal, err := readReviewProposal(*proposalPath)
	if err != nil {
		return err
	}
	next, coverage, err := obligations.RecordReview(index, catalog, registry, proposal)
	if err != nil {
		return fmt.Errorf("record obligation review: %w", err)
	}
	// Persist the validated append-only ledger before publishing its derived summary.
	if err := obligations.SaveRegistry(*registryPath, next); err != nil {
		return err
	}
	if strings.TrimSpace(*coveragePath) != "" {
		data, err := json.MarshalIndent(coverage, "", "  ")
		if err != nil {
			return fmt.Errorf("encode binding coverage: %w", err)
		}
		if err := writeFile(*coveragePath, data); err != nil {
			return err
		}
	}
	fmt.Printf("Recorded %s revision for %s. Blocking bindings: %d; advisory bindings: %d; review required: %d; stale: %d.\n",
		proposal.Decision, proposal.CandidateID, coverage.BlockingBindingCount, coverage.AdvisoryBindingCount, coverage.ReviewRequiredCount, coverage.StaleCount)
	return nil
}

func runValidateObligationReviews(args []string) error {
	fs := flag.NewFlagSet("validate-obligation-reviews", flag.ContinueOnError)
	indexPath := fs.String("index", "adr/obligation-candidates.json", "latest candidate index JSON")
	catalogPath := fs.String("requirements", "adr/requirements.json", "requirements catalog JSON")
	registryPath := fs.String("registry", "adr/obligation-reviews.json", "append-only review registry")
	coveragePath := fs.String("json-out", "", "binding coverage JSON output (default: stdout)")
	strict := fs.Bool("strict", false, "fail on stale approvals or orphaned candidate history")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("validate-obligation-reviews accepts no positional arguments")
	}
	index, err := readObligationIndex(*indexPath)
	if err != nil {
		return err
	}
	catalog, err := assessment.LoadCatalog(*catalogPath)
	if err != nil {
		return err
	}
	registry, err := obligations.LoadRegistry(*registryPath)
	if err != nil {
		return err
	}
	coverage, err := obligations.ValidateRegistry(index, catalog, registry)
	if err != nil {
		return fmt.Errorf("validate obligation review registry: %w", err)
	}
	data, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		return fmt.Errorf("encode binding coverage: %w", err)
	}
	if strings.TrimSpace(*coveragePath) != "" {
		if err := writeFile(*coveragePath, data); err != nil {
			return err
		}
	} else {
		fmt.Println(string(data))
	}
	if *strict {
		if _, err := obligations.ValidateActiveRegistry(index, catalog, registry); err != nil {
			return fmt.Errorf("strict obligation review validation failed: %w", err)
		}
	}
	return nil
}

// runAssessObligations joins a repository assessment with reviewed candidate
// obligations. It deliberately consumes repository-mode assessment evidence
// rather than silently rerunning scanners with potentially different inputs.
