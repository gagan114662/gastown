package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/ctxstack"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	summariesRole    string
	summariesRig     string
	summariesAgent   string
	summariesBead    string
	summariesSource  string
	summariesSession string
	summariesLimit   int
	summariesJSON    bool
)

func init() {
	summariesCmd.Flags().StringVar(&summariesRole, "role", "", "Filter by role")
	summariesCmd.Flags().StringVar(&summariesRig, "rig", "", "Filter by rig")
	summariesCmd.Flags().StringVar(&summariesAgent, "agent", "", "Filter by agent")
	summariesCmd.Flags().StringVar(&summariesBead, "bead", "", "Filter by work bead")
	summariesCmd.Flags().StringVar(&summariesSource, "source", "", "Filter by summary source")
	summariesCmd.Flags().StringVar(&summariesSession, "session", "", "Filter by session id")
	summariesCmd.Flags().IntVar(&summariesLimit, "limit", 10, "Maximum summaries to return")
	summariesCmd.Flags().BoolVar(&summariesJSON, "json", false, "Output as JSON")
	summariesCmd.GroupID = GroupDiag
	rootCmd.AddCommand(summariesCmd)
}

var summariesCmd = &cobra.Command{
	Use:   "summaries",
	Short: "Inspect warm session summaries",
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot := detectTownRootFromCwd()
		if townRoot == "" {
			return fmt.Errorf("not in a Gas Town workspace")
		}
		store, err := openContextStore(townRoot)
		if err != nil {
			return err
		}
		summaries, err := store.ListSessionSummaries(ctxstack.SummaryFilter{
			Role:     summariesRole,
			Rig:      summariesRig,
			Agent:    summariesAgent,
			WorkBead: summariesBead,
			Source:   summariesSource,
			Session:  summariesSession,
			Limit:    summariesLimit,
		})
		if err != nil {
			return err
		}
		if summariesJSON {
			data, err := json.MarshalIndent(summaries, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		if len(summaries) == 0 {
			fmt.Println("No summaries found.")
			return nil
		}
		fmt.Printf("%s (%d)\n\n", style.Bold.Render("Session Summaries"), len(summaries))
		for _, summary := range summaries {
			fmt.Printf("%s\n", style.Bold.Render(renderSummaryHeadline(summary)))
			fmt.Printf("  %s\n", trimContextText(summary.Summary, 240))
			if summary.NextSteps != "" {
				fmt.Printf("  next: %s\n", trimContextText(summary.NextSteps, 140))
			}
			if summary.Blockers != "" {
				fmt.Printf("  blockers: %s\n", trimContextText(summary.Blockers, 140))
			}
			fmt.Println()
		}
		return nil
	},
}
