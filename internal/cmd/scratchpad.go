package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/ctxstack"
)

var (
	scratchpadSession string
	scratchpadKind    string
	scratchpadLimit   int
)

func init() {
	scratchpadCmd.PersistentFlags().StringVar(&scratchpadSession, "session", "", "Session id (default: current session)")
	scratchpadCmd.GroupID = GroupWork
	scratchpadAppendCmd.Flags().StringVar(&scratchpadKind, "kind", "note", "Scratchpad entry kind")
	scratchpadShowCmd.Flags().IntVar(&scratchpadLimit, "limit", 50, "Maximum entries to show")
	scratchpadCmd.AddCommand(scratchpadAppendCmd)
	scratchpadCmd.AddCommand(scratchpadShowCmd)
	scratchpadCmd.AddCommand(scratchpadClearCmd)
	scratchpadCmd.AddCommand(scratchpadCompactCmd)
	rootCmd.AddCommand(scratchpadCmd)
}

var scratchpadCmd = &cobra.Command{
	Use:   "scratchpad",
	Short: "Manage hidden session scratchpads",
}

var scratchpadAppendCmd = &cobra.Command{
	Use:   "append <text>",
	Short: "Append a scratchpad note",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, sessionID, err := currentScratchpadStore()
		if err != nil {
			return err
		}
		return store.AddScratchpadEntry(ctxstack.ScratchpadEntry{
			SessionID: sessionID,
			Kind:      scratchpadKind,
			Text:      strings.Join(args, " "),
		})
	},
}

var scratchpadShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show scratchpad entries for a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, sessionID, err := currentScratchpadStore()
		if err != nil {
			return err
		}
		entries, err := store.ListScratchpad(sessionID, scratchpadLimit)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("Scratchpad empty.")
			return nil
		}
		for _, entry := range entries {
			fmt.Printf("[%03d] %s %s\n", entry.Seq, entry.CreatedAt.Local().Format("2006-01-02 15:04"), entry.Kind)
			fmt.Printf("  %s\n", entry.Text)
		}
		return nil
	},
}

var scratchpadClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear scratchpad entries for a session",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, sessionID, err := currentScratchpadStore()
		if err != nil {
			return err
		}
		return store.ClearScratchpad(sessionID)
	},
}

var scratchpadCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact scratchpad entries into a warm summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, sessionID, err := currentScratchpadStore()
		if err != nil {
			return err
		}
		entries, err := store.ListScratchpad(sessionID, 200)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("Scratchpad empty.")
			return nil
		}
		var lines []string
		for _, entry := range entries {
			lines = append(lines, entry.Kind+": "+entry.Text)
		}
		body := strings.Join(lines, "\n")
		townRoot, role, rig, agent, _, workBead, source := currentContextSessionSummarySource("scratchpad_compact")
		if err := recordContextSummary(townRoot, role, rig, agent, sessionID, workBead, source, body, "scratchpad", "compact"); err != nil {
			return err
		}
		if err := store.ClearScratchpad(sessionID); err != nil {
			return err
		}
		fmt.Println("Scratchpad compacted into warm memory.")
		return nil
	},
}

func currentScratchpadStore() (*ctxstack.Store, string, error) {
	townRoot := detectTownRootFromCwd()
	if townRoot == "" {
		return nil, "", fmt.Errorf("not in a Gas Town workspace")
	}
	store, err := openContextStore(townRoot)
	if err != nil {
		return nil, "", err
	}
	sessionID := scratchpadSession
	if sessionID == "" {
		sessionID = currentContextSessionID()
	}
	if sessionID == "" {
		return nil, "", fmt.Errorf("no session id available")
	}
	return store, sessionID, nil
}
