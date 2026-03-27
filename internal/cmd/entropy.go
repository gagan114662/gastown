package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/ctxstack"
	"github.com/steveyegge/gastown/internal/tmux"
	"golang.org/x/term"
)

var (
	entropyJSON      bool
	entropyGuard     bool
	entropySession   string
	entropyAction    string
	entropyRole      string
	entropyRig       string
	entropyBead      string
	entropyUsed      int
	entropyMaxTokens int
)

func init() {
	entropyCmd.Flags().BoolVar(&entropyJSON, "json", false, "Output as JSON")
	entropyCmd.Flags().BoolVar(&entropyGuard, "guard", false, "Run as a guard and emit threshold guidance")
	entropyCmd.Flags().StringVar(&entropySession, "session", "", "Session id (default: current session)")
	entropyCmd.Flags().StringVar(&entropyAction, "action", "", "Action label to record with the sample")
	entropyCmd.Flags().StringVar(&entropyRole, "role", "", "Role override")
	entropyCmd.Flags().StringVar(&entropyRig, "rig", "", "Rig override")
	entropyCmd.Flags().StringVar(&entropyBead, "bead", "", "Work bead override")
	entropyCmd.Flags().IntVar(&entropyUsed, "used-tokens", 0, "Override used token count")
	entropyCmd.Flags().IntVar(&entropyMaxTokens, "max-tokens", 0, "Override max token count")
	entropyCmd.GroupID = GroupDiag
	rootCmd.AddCommand(entropyCmd)
}

var entropyCmd = &cobra.Command{
	Use:   "entropy",
	Short: "Diagnose context drift and budget pressure",
	RunE: func(cmd *cobra.Command, args []string) error {
		townRoot := detectTownRootFromCwd()
		if townRoot == "" {
			return fmt.Errorf("not in a Gas Town workspace")
		}
		role := entropyRole
		if role == "" {
			role = currentContextRole()
		}
		rigName := entropyRig
		if rigName == "" {
			rigName = currentContextRig()
		}
		workBead := entropyBead
		if workBead == "" {
			workBead = currentContextBead()
		}
		sessionID := entropySession
		if sessionID == "" {
			sessionID = currentContextSessionID()
		}
		if sessionID == "" {
			return fmt.Errorf("no session id available")
		}

		store, err := openContextStore(townRoot)
		if err != nil {
			return err
		}
		settings := loadContextSettings(townRoot, rigName)
		caps := resolveRuntimeContextCapabilities(townRoot, rigName, role)
		maxTokens := entropyMaxTokens
		if maxTokens <= 0 {
			maxTokens = settings.EffectiveMaxTokens(caps)
		}
		usage, err := entropyUsage(maxTokens)
		if err != nil {
			return err
		}

		entries, err := store.ListScratchpad(sessionID, 200)
		if err != nil {
			return err
		}
		restarts, repeatHits, toolLoops := entropyHistorySignals(store, role, rigName, workBead, sessionID)
		lastActivity, _ := store.LatestActivityAt(sessionID)
		minutesNoProgress := 0.0
		if !lastActivity.IsZero() {
			minutesNoProgress = time.Since(lastActivity).Minutes()
		}
		scratchpadChars := 0
		for _, entry := range entries {
			scratchpadChars += len(entry.Text)
		}

		inputs := ctxstack.EntropyInputs{
			ContextUsage:       usageRatio(usage),
			RestartCount:       restarts,
			RepeatedPromptHits: repeatHits,
			ToolLoopHits:       toolLoops,
			ScratchpadEntries:  len(entries),
			ScratchpadChars:    scratchpadChars,
			MinutesNoProgress:  minutesNoProgress,
		}
		sample := ctxstack.ScoreEntropy(inputs)
		sample.SessionID = sessionID
		sample.ContextUsage = usageRatio(usage)
		sample.Action = entropyAction
		sample.CreatedAt = time.Now().UTC()
		sample.Band = ctxstack.BandFor(sample, settings.Thresholds)

		if err := store.PutEntropySample(sample); err != nil {
			return err
		}

		if entropyJSON {
			payload := map[string]any{
				"sample": sample,
				"usage":  usage,
			}
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
		} else {
			printEntropyHuman(sample, usage)
		}

		if entropyGuard {
			return runEntropyGuard(sample, usage, settings, role, store, townRoot, rigName, workBead)
		}
		return nil
	},
}

func entropyUsage(maxTokens int) (*ctxstack.UsageSample, error) {
	if entropyUsed > 0 {
		return &ctxstack.UsageSample{
			UsedTokens: entropyUsed,
			MaxTokens:  maxTokens,
			Ratio:      float64(entropyUsed) / float64(maxTokens),
			Source:     "flag",
		}, nil
	}
	cwd, _ := os.Getwd()
	return ctxstack.InferUsage(cwd, maxTokens)
}

func usageRatio(sample *ctxstack.UsageSample) float64 {
	if sample == nil {
		return 0
	}
	return sample.Ratio
}

func entropyHistorySignals(store *ctxstack.Store, role, rigName, workBead, sessionID string) (restarts, repeatHits, toolLoops int) {
	summaries, err := store.ListSessionSummaries(ctxstack.SummaryFilter{
		Role:     role,
		Rig:      rigName,
		WorkBead: workBead,
		Limit:    12,
	})
	if err != nil {
		return 0, 0, 0
	}
	seenSummaries := map[string]int{}
	for _, summary := range summaries {
		if strings.Contains(summary.Source, "handoff") || strings.Contains(summary.Source, "cycle") {
			restarts++
		}
		normalized := strings.Join(strings.Fields(strings.ToLower(summary.Summary)), " ")
		if normalized == "" {
			continue
		}
		seenSummaries[normalized]++
		if seenSummaries[normalized] > 1 {
			repeatHits++
		}
		if strings.Contains(strings.ToLower(summary.Blockers), "no progress") || strings.Contains(strings.ToLower(summary.Blockers), "stuck") {
			toolLoops++
		}
	}
	if latest, err := store.LatestEntropySample(sessionID); err == nil && latest != nil {
		if strings.Contains(latest.Action, "guard") && latest.Band != ctxstack.EntropyBandHealthy {
			restarts++
		}
	}
	return restarts, repeatHits, toolLoops
}

func printEntropyHuman(sample ctxstack.EntropySample, usage *ctxstack.UsageSample) {
	if usage != nil {
		fmt.Printf("Usage: %.0f%% (%d/%d tokens)\n", usage.Ratio*100, usage.UsedTokens, usage.MaxTokens)
	}
	fmt.Printf("Entropy: %.2f [%s]\n", sample.Score, sample.Band)
	if len(sample.Reasons) > 0 {
		fmt.Printf("Reasons: %s\n", strings.Join(sample.Reasons, "; "))
	}
}

func runEntropyGuard(sample ctxstack.EntropySample, usage *ctxstack.UsageSample, settings ctxstack.Settings, role string, store *ctxstack.Store, townRoot, rigName, workBead string) error {
	prefix := "Context/entropy"
	if usage != nil && usage.Ratio >= settings.Thresholds.HardUsage {
		fmt.Fprintf(os.Stderr, "%s hard threshold exceeded\n", prefix)
	} else if sample.Band == ctxstack.EntropyBandSoft {
		fmt.Fprintf(os.Stderr, "%s soft threshold exceeded\n", prefix)
	} else if sample.Band == ctxstack.EntropyBandWarn {
		fmt.Fprintf(os.Stderr, "%s warning threshold exceeded\n", prefix)
	}

	if shouldAutoRecover(role, settings) && (sample.Band == ctxstack.EntropyBandSoft || sample.Band == ctxstack.EntropyBandHard) {
		action := "handoff"
		args := []string{"handoff", "--auto", "--reason", "context-budget", "--yes"}
		if tmux.IsInsideTmux() {
			action = "cycle"
			args = []string{"handoff", "--cycle", "--reason", "context-budget", "--yes"}
		}
		sample.Action = "guard:" + action
		_ = store.PutEntropySample(sample)
		_ = recordContextSummary(townRoot, role, rigName, currentContextAgent(), currentContextSessionID(), workBead, "entropy_guard", fmt.Sprintf("band=%s score=%.2f", sample.Band, sample.Score), "entropy", sample.Band)
		cmd := exec.Command("gt", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		return nil
	}
	if sample.Band == ctxstack.EntropyBandHard {
		return NewSilentExit(2)
	}
	return nil
}

func shouldAutoRecover(role string, settings ctxstack.Settings) bool {
	if !settings.Recovery.Enabled || !settings.Recovery.AutonomousAutoRecover {
		return false
	}
	if settings.Recovery.InteractiveWarnOnly && term.IsTerminal(int(os.Stdin.Fd())) {
		return false
	}
	switch role {
	case "mayor", "deacon", "witness", "refinery", "polecat", "crew", "dog":
		return true
	default:
		return false
	}
}
