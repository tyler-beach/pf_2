package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := commandContext()
	defer stop()

	var err error
	switch os.Args[1] {
	case "validate":
		err = runValidate(os.Args[2:])
	case "assess":
		err = runAssessCtx(ctx, os.Args[2:], false)
	case "assess-repo":
		err = runAssessCtx(ctx, os.Args[2:], true)
	case "ingest-adrs":
		err = runIngestADRs(os.Args[2:])
	case "extract-obligations":
		err = runExtractObligations(ctx, os.Args[2:])
	case "assist-obligations":
		err = runAssistObligations(ctx, os.Args[2:])
	case "proposal-to-review":
		err = runProposalToReview(os.Args[2:])
	case "accept-assist-proposal":
		err = runAcceptAssistProposal(os.Args[2:])
	case "low-confidence-checklist":
		err = runLowConfidenceChecklist(os.Args[2:])
	case "scan-opengrep":
		err = runOpenGrepScan(ctx, os.Args[2:])
	case "scan-checkov":
		err = runCheckovScan(ctx, os.Args[2:])
	case "plan-repo":
		err = runPlanRepo(os.Args[2:])
	case "verify-plan":
		err = runVerifyPlan(os.Args[2:])
	case "execute-plan":
		err = runExecutePlan(ctx, os.Args[2:])
	case "assert-issue-verdict":
		err = runAssertIssueVerdict(os.Args[2:])
	case "review-init":
		err = runInitReviewRegistry(os.Args[2:])
	case "record-obligation-review":
		err = runRecordObligationReview(os.Args[2:])
	case "validate-obligation-reviews":
		err = runValidateObligationReviews(os.Args[2:])
	case "assess-obligations":
		err = runAssessObligations(os.Args[2:])
	case "workshop-emit":
		err = runWorkshopEmit(os.Args[2:])
	case "specification-check":
		err = runSpecificationCheck(os.Args[2:])
	case "parse-pr-url":
		err = runParsePRURL(os.Args[2:])
	case "parse-pr-scan-issue":
		err = runParsePRScanIssue(os.Args[2:])
	case "assessment-to-rdjson":
		err = runAssessmentToRDJSON(ctx, os.Args[2:])
	case "version":
		fmt.Println("adrctl 0.1.0")
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "adrctl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: adrctl <validate|assess|assess-repo|ingest-adrs|extract-obligations|assist-obligations|proposal-to-review|accept-assist-proposal|low-confidence-checklist|scan-opengrep|scan-checkov|plan-repo|verify-plan|execute-plan|assert-issue-verdict|review-init|record-obligation-review|validate-obligation-reviews|assess-obligations|workshop-emit|specification-check|parse-pr-url|parse-pr-scan-issue|assessment-to-rdjson|version> [flags]")
}
