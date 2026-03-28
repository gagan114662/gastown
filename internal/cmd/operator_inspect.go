package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/controlplane"
	"github.com/steveyegge/gastown/internal/operatorview"
	"github.com/steveyegge/gastown/internal/workspace"
)

var inspectCmd = &cobra.Command{
	Use:     "inspect",
	GroupID: GroupDiag,
	Short:   "Inspect reconciled runtime state",
}

var inspectTownCmd = &cobra.Command{
	Use:   "town",
	Short: "Inspect the reconciled town snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return err
		}
		snapshot, err := operatorview.LoadTownSnapshot(townRoot)
		if err != nil {
			return err
		}
		if commandJSON(cmd) {
			return encodeJSON(snapshot)
		}

		fmt.Printf("Town: %s\nStatus: %s\nReason: %s\nAgents: %d\nIncidents: %d\nLeases: %d\nRespawns: %d\nRedispatches: %d\nCleanup States: %d\nDependencies: %d\n",
			snapshot.TownRoot, snapshot.Status, snapshot.StatusReason, len(snapshot.Agents), len(snapshot.Incidents),
			len(snapshot.Leases), len(snapshot.Respawns), len(snapshot.Redispatches), len(snapshot.CleanupStates), len(snapshot.Dependencies))
		if len(snapshot.Conflicts) > 0 {
			fmt.Println("Conflicts:")
			for _, conflict := range snapshot.Conflicts {
				fmt.Printf("- %s\n", conflict)
			}
		}
		return nil
	},
}

var inspectAgentCmd = &cobra.Command{
	Use:   "agent <id>",
	Short: "Inspect a reconciled agent/session snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return err
		}
		snapshot, err := operatorview.LoadAgentSnapshot(townRoot, args[0])
		if err != nil {
			return err
		}
		if snapshot == nil {
			return fmt.Errorf("agent not found: %s", args[0])
		}
		if commandJSON(cmd) {
			return encodeJSON(snapshot)
		}

		fmt.Printf("Agent: %s\nSession: %s\nStatus: %s\nReason: %s\nAgreement: %s\n",
			snapshot.AgentID, snapshot.Session, snapshot.Status, snapshot.StatusReason, snapshot.SourceAgreement)
		if snapshot.Lease != nil {
			fmt.Printf("Lease: %s (%s)\n", snapshot.Lease.Service, snapshot.Lease.Status)
		}
		if snapshot.Respawn != nil {
			fmt.Printf("Respawn: %d/%d blocked=%t\n", snapshot.Respawn.Count, snapshot.Respawn.MaxCount, snapshot.Respawn.Blocked)
		}
		if snapshot.Redispatch != nil {
			fmt.Printf("Redispatch: attempts=%d action=%s escalated=%t\n", snapshot.Redispatch.AttemptCount, snapshot.Redispatch.LastAction, snapshot.Redispatch.Escalated)
		}
		if snapshot.Cleanup != nil {
			fmt.Printf("Cleanup: %s blocker=%s attempts=%d\n", snapshot.Cleanup.Status, snapshot.Cleanup.Blocker, snapshot.Cleanup.AttemptCount)
		}
		if len(snapshot.Conflicts) > 0 {
			fmt.Println("Conflicts:")
			for _, conflict := range snapshot.Conflicts {
				fmt.Printf("- %s\n", conflict)
			}
		}
		if len(snapshot.Decisions) > 0 {
			fmt.Println("Recent Decisions:")
			for _, decision := range snapshot.Decisions {
				fmt.Printf("- %s %s %s\n", decision.Timestamp, decision.Kind, decision.Reason)
			}
		}
		return nil
	},
}

var operatorEventsCmd = &cobra.Command{
	Use:     "events",
	GroupID: GroupDiag,
	Short:   "Inspect canonical operator events",
}

var operatorEventsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Show recent canonical events",
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return err
		}
		store, err := controlplane.Open(townRoot)
		if err != nil {
			return err
		}
		events, err := store.ListEvents(commandLimit(cmd, 20))
		if err != nil {
			return err
		}
		if commandJSON(cmd) {
			return encodeJSON(events)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tKIND\tACTOR\tSESSION\tOUTCOME\tREASON")
		for _, event := range events {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				event.Timestamp, event.Kind, event.Actor, event.Session, event.Outcome, event.Reason)
		}
		return w.Flush()
	},
}

var incidentsCmd = &cobra.Command{
	Use:     "incidents",
	GroupID: GroupDiag,
	Short:   "Show incident candidates from the canonical event stream",
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return err
		}
		store, err := controlplane.Open(townRoot)
		if err != nil {
			return err
		}
		incidents, err := store.ListIncidents(commandLimit(cmd, 20))
		if err != nil {
			return err
		}
		if commandJSON(cmd) {
			return encodeJSON(incidents)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tSEVERITY\tKIND\tSESSION\tSUMMARY")
		for _, incident := range incidents {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				incident.Timestamp, incident.Severity, incident.Kind, incident.Session, incident.Summary)
		}
		return w.Flush()
	},
}

func init() {
	inspectTownCmd.Flags().Bool("json", false, "Output as JSON")
	inspectAgentCmd.Flags().Bool("json", false, "Output as JSON")
	operatorEventsTailCmd.Flags().Bool("json", false, "Output as JSON")
	incidentsCmd.Flags().Bool("json", false, "Output as JSON")
	operatorEventsTailCmd.Flags().IntP("limit", "n", 20, "Number of events to show")
	incidentsCmd.Flags().IntP("limit", "n", 20, "Number of incidents to show")

	inspectCmd.AddCommand(inspectTownCmd)
	inspectCmd.AddCommand(inspectAgentCmd)
	operatorEventsCmd.AddCommand(operatorEventsTailCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(operatorEventsCmd)
	rootCmd.AddCommand(incidentsCmd)
}

func encodeJSON(value interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func commandJSON(cmd *cobra.Command) bool {
	value, err := cmd.Flags().GetBool("json")
	return err == nil && value
}

func commandLimit(cmd *cobra.Command, fallback int) int {
	value, err := cmd.Flags().GetInt("limit")
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
