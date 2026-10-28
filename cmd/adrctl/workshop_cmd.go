package main

import (
	"flag"
	"fmt"

	"github.com/tyler-beach/pf_2/internal/workshop"
)

func runWorkshopEmit(args []string) error {
	fs := flag.NewFlagSet("workshop-emit", flag.ContinueOnError)
	outDir := fs.String("out-dir", "workshop-snapshot", "workshop snapshot output directory")
	runID := fs.String("run-id", "", "workflow run id")
	workflow := fs.String("workflow", "local", "workflow name")
	gitSHA := fs.String("git-sha", "", "git commit SHA")
	htmlURL := fs.String("html-url", "", "workflow run HTML URL")
	providerMode := fs.String("provider-mode", "", "offline|live assist mode tag")
	candidates := fs.String("candidates", "", "obligation candidates JSON")
	coverage := fs.String("coverage", "", "obligation review coverage JSON")
	assessmentPath := fs.String("assessment", "", "assessment JSON")
	submissions := fs.String("submissions", "", "workshop submissions index JSON (rolls ADR flagging across assessment_path files)")
	proposals := fs.String("proposals", "", "LLM assist proposals JSON")
	registryPath := fs.String("registry", "", "obligation review registry JSON (for accept_rate)")
	harness := fs.String("harness", "", "reuse/corpus harness metrics JSON")
	promptfooPath := fs.String("promptfoo", "", "Promptfoo results JSON")
	corpusLabels := fs.String("corpus-labels", "", "corpus labels.json")
	enforceAssessLLM := fs.Bool("enforce-assess-llm", false, "fail if assessment runtime_llm_calls != 0")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("workshop-emit accepts no positional arguments")
	}
	latest, err := workshop.Emit(workshop.EmitOptions{
		OutDir: *outDir,
		Run: workshop.RunIdentity{
			RunID:        *runID,
			Workflow:     *workflow,
			GitSHA:       *gitSHA,
			HTMLURL:      *htmlURL,
			ProviderMode: *providerMode,
		},
		CandidatesPath:       *candidates,
		CoveragePath:         *coverage,
		AssessmentPath:       *assessmentPath,
		SubmissionsIndexPath: *submissions,
		ProposalsPath:        *proposals,
		RegistryPath:         *registryPath,
		HarnessPath:          *harness,
		PromptfooPath:        *promptfooPath,
		CorpusLabelsPath:     *corpusLabels,
		EnforceAssessLLM:     *enforceAssessLLM,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Workshop snapshot %s written to %s (%s)\n", latest.Run.RunID, *outDir, latest.Summary)
	return nil
}
