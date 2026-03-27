package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/events"
	gtHealth "github.com/steveyegge/gastown/internal/health"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	patrolRemediateRig            string
	patrolRemediateJSON           bool
	patrolRemediateDryRun         bool
	patrolRemediateAnomalyWindow  time.Duration
	patrolRemediateMinActivity    int
)

type patrolRemediationResult struct {
	Target string                    `json:"target"`
	State  string                    `json:"state"`
	Action gtHealth.RemediationAction `json:"action"`
	Reason string                    `json:"reason"`
	Error  string                    `json:"error,omitempty"`
}

type patrolRemediationSummary struct {
	Rig       string                    `json:"rig"`
	Results   []patrolRemediationResult `json:"results"`
	Anomaly   *gtHealth.AnomalySummary  `json:"anomaly,omitempty"`
	DryRun    bool                      `json:"dry_run"`
}

var patrolRemediateCmd = &cobra.Command{
	Use:   "remediate",
	Short: "Apply autonomous remediation to unhealthy rig agents",
	Long: `Detect agents needing intervention and apply policy-driven actions.

The remediation policy is:
  stalled          -> nudge once, then handoff on repeat
  gupp violation   -> handoff, then escalate on repeat
  zombie           -> trigger witness restart/cleanup scan
  anomaly window   -> dispatch anomaly-investigation dog

This command is intended for Deacon/Witness patrol loops so the feed is no
longer the only place these problems surface.`,
	RunE: runPatrolRemediate,
}

func init() {
	patrolRemediateCmd.Flags().StringVar(&patrolRemediateRig, "rig", "", "Rig to remediate (default: infer from cwd or GT_RIG)")
	patrolRemediateCmd.Flags().BoolVar(&patrolRemediateJSON, "json", false, "Output JSON")
	patrolRemediateCmd.Flags().BoolVar(&patrolRemediateDryRun, "dry-run", false, "Show actions without executing them")
	patrolRemediateCmd.Flags().DurationVar(&patrolRemediateAnomalyWindow, "anomaly-window", 15*time.Minute, "Window used for anomaly detection")
	patrolRemediateCmd.Flags().IntVar(&patrolRemediateMinActivity, "anomaly-min-activity", 12, "Minimum activity count before anomaly dispatch")
	patrolCmd.AddCommand(patrolRemediateCmd)
}

func runPatrolRemediate(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName := patrolRemediateRig
	if rigName == "" {
		rigName = os.Getenv("GT_RIG")
		if rigName == "" {
			rigName, err = inferRigFromCwd(townRoot)
			if err != nil {
				return fmt.Errorf("could not determine rig: %w\nUse --rig to specify", err)
			}
		}
	}
	rigPath := filepath.Join(townRoot, rigName)

	detector := gtHealth.NewAgentDetector(beads.New(beads.ResolveBeadsDir(rigPath)))
	problems, err := detector.CheckAll()
	if err != nil {
		return err
	}
	problems = witness.FilterProblemsForRig(problems, rigName)

	state, err := gtHealth.LoadRemediationState(townRoot)
	if err != nil {
		return fmt.Errorf("loading remediation state: %w", err)
	}
	summary := patrolRemediationSummary{Rig: rigName, DryRun: patrolRemediateDryRun}

	needsZombieScan := false
	for _, problem := range problems {
		if !isAutoRemediationRole(problem.Role) {
			continue
		}
		record := state.Observe(problem)
		decision := deacon.DecideRemediation(problem, record)
		target := remediationTarget(problem)
		result := patrolRemediationResult{
			Target: target,
			State:  problem.State.String(),
			Action: decision.Action,
			Reason: decision.Reason,
		}
		if decision.Action == gtHealth.ActionNone {
			continue
		}

		if patrolRemediateDryRun {
			summary.Results = append(summary.Results, result)
			continue
		}

		switch decision.Action {
		case gtHealth.ActionNudge:
			err = runInternalGT(townRoot, "nudge", target, fmt.Sprintf("Patrol detected a stalled session (%s). Check your hook and continue.", problem.DurationDisplay()), "--mode", "immediate")
		case gtHealth.ActionHandoff:
			err = runInternalGT(townRoot, "handoff", target, "--watch=false", "--reason", "patrol-remediate", "--message", decision.Reason)
		case gtHealth.ActionEscalate:
			err = runInternalGT(townRoot, "escalate", fmt.Sprintf("Repeated GUPP violation on %s", target), "--severity", "high", "--reason", decision.Reason)
		case gtHealth.ActionRestart:
			needsZombieScan = true
		}
		if err != nil {
			result.Error = err.Error()
		} else if decision.Action != gtHealth.ActionRestart {
			state.RecordAction(problem, decision.Action)
			_ = events.LogFeed(events.TypeAutoRemediation, "deacon", map[string]interface{}{
				"rig":    rigName,
				"target": target,
				"state":  problem.State.String(),
				"action": decision.Action,
				"reason": decision.Reason,
			})
		}
		summary.Results = append(summary.Results, result)
	}

	if needsZombieScan {
		zombieResult := patrolRemediationResult{
			Target: rigName,
			State:  gtHealth.StateZombie.String(),
			Action: gtHealth.ActionRestart,
			Reason: "run witness zombie scan and restart pipeline",
		}
		if patrolRemediateDryRun {
			summary.Results = append(summary.Results, zombieResult)
		} else {
			err := runInternalGT(townRoot, "patrol", "scan", "--rig", rigName)
			if err != nil {
				zombieResult.Error = err.Error()
			} else {
				_ = events.LogFeed(events.TypeAutoRemediation, "deacon", map[string]interface{}{
					"rig":    rigName,
					"target": rigName,
					"state":  gtHealth.StateZombie.String(),
					"action": gtHealth.ActionRestart,
					"reason": zombieResult.Reason,
				})
			}
			summary.Results = append(summary.Results, zombieResult)
		}
	}

	anomaly, err := gtHealth.DetectEventAnomaly(townRoot, rigName, patrolRemediateAnomalyWindow, patrolRemediateMinActivity)
	if err != nil {
		return fmt.Errorf("detecting anomaly: %w", err)
	}
	if anomaly != nil {
		summary.Anomaly = anomaly
		if !patrolRemediateDryRun {
			if err := runInternalGT(townRoot, "dog", "dispatch", "--plugin", "anomaly-investigation", "--rig", rigName, "--create"); err != nil {
				summary.Results = append(summary.Results, patrolRemediationResult{
					Target: rigName,
					State:  "anomaly",
					Action: gtHealth.ActionInvestigate,
					Reason: anomaly.Reason,
					Error:  err.Error(),
				})
			} else {
				_ = events.LogFeed(events.TypeAnomalyInvestigation, "deacon", map[string]interface{}{
					"rig":            rigName,
					"activity_count": anomaly.ActivityCount,
					"done_count":     anomaly.DoneCount,
					"reason":         anomaly.Reason,
				})
			}
		}
	}

	if !patrolRemediateDryRun {
		if err := gtHealth.SaveRemediationState(townRoot, state); err != nil {
			return fmt.Errorf("saving remediation state: %w", err)
		}
	}

	if patrolRemediateJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	if len(summary.Results) == 0 && summary.Anomaly == nil {
		fmt.Printf("%s No patrol remediation needed for %s\n", style.Dim.Render("○"), rigName)
		return nil
	}

	fmt.Printf("%s Patrol remediation for %s\n", style.Bold.Render("✓"), rigName)
	for _, result := range summary.Results {
		line := fmt.Sprintf("  %s %s -> %s (%s)", style.Bold.Render("·"), result.Target, result.Action, result.Reason)
		if result.Error != "" {
			line += fmt.Sprintf(" [error: %s]", result.Error)
		}
		fmt.Println(line)
	}
	if summary.Anomaly != nil {
		fmt.Printf("  %s anomaly: %s\n", style.Bold.Render("·"), summary.Anomaly.Reason)
	}
	return nil
}

func isAutoRemediationRole(role string) bool {
	switch role {
	case constants.RolePolecat, constants.RoleWitness, constants.RoleRefinery, constants.RoleDeacon:
		return true
	default:
		return false
	}
}

func remediationTarget(problem *gtHealth.ProblemAgent) string {
	switch problem.Role {
	case constants.RoleWitness:
		return fmt.Sprintf("%s/witness", problem.Rig)
	case constants.RoleRefinery:
		return fmt.Sprintf("%s/refinery", problem.Rig)
	case constants.RoleDeacon:
		return "deacon"
	default:
		return fmt.Sprintf("%s/%s", problem.Rig, strings.ToLower(problem.Name))
	}
}

func runInternalGT(townRoot string, args ...string) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(bin, args...)
	command.Dir = townRoot
	command.Stdout = nil
	command.Stderr = os.Stderr
	return command.Run()
}
