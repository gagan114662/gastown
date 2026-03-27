package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

var wlVerifyCmd = &cobra.Command{
	Use:   "verify <completion-id>",
	Short: "Verify completion evidence and persist the validation result",
	Args:  cobra.ExactArgs(1),
	RunE:  runWlVerify,
}

func init() {
	wlCmd.AddCommand(wlVerifyCmd)
}

func runWlVerify(cmd *cobra.Command, args []string) error {
	completionID := args[0]
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	cfg, err := wasteland.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading wasteland config: %w", err)
	}
	if err := requireWastelandTier(townRoot, cfg, wasteland.TierWarChief, "verify completions"); err != nil {
		return err
	}

	var completion *doltserver.CompletionRecord
	if doltserver.DatabaseExists(townRoot, doltserver.WLCommonsDB) {
		completion, err = doltserver.QueryCompletion(townRoot, completionID)
		if err != nil {
			return err
		}
	} else {
		if cfg.LocalDir == "" {
			return fmt.Errorf("no local wasteland clone configured")
		}
		completion, err = queryCompletionInLocalClone(cfg.LocalDir, completionID)
		if err != nil {
			return err
		}
	}

	assessment, err := wasteland.AnalyzeEvidence(completion.Evidence)
	if err != nil {
		return err
	}
	if assessment.Status != wasteland.ValidationVerified {
		return fmt.Errorf("evidence cannot be auto-verified; keep status as %s", assessment.Status)
	}

	if doltserver.DatabaseExists(townRoot, doltserver.WLCommonsDB) {
		if err := doltserver.UpdateCompletionValidation(townRoot, completionID, cfg.RigHandle, assessment); err != nil {
			return err
		}
	} else {
		if err := updateCompletionValidationInLocalClone(cfg.LocalDir, completionID, cfg.RigHandle, assessment); err != nil {
			return err
		}
	}

	fmt.Printf("%s Verified completion %s\n", style.Success.Render("✓"), completionID)
	fmt.Printf("  Evidence type: %s\n", assessment.Type)
	fmt.Printf("  Validation: %s\n", assessment.Status)
	fmt.Printf("  Verified by: %s\n", cfg.RigHandle)
	return nil
}
