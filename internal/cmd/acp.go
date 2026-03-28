package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/acp"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	acpVerifyJSON    bool
	acpVerifyTimeout time.Duration
	acpVerifyPrompt  string
)

var acpCmd = &cobra.Command{
	Use:     "acp",
	GroupID: GroupConfig,
	Short:   "ACP protocol tools",
	RunE:    requireSubcommand,
}

var acpVerifyCmd = &cobra.Command{
	Use:   "verify [agent-alias-or-command]",
	Short: "Verify an ACP runtime over JSON-RPC/stdin",
	Long: `Verify that a runtime implements the ACP v1 JSON-RPC/stdin contract.

When run inside a Gas Town workspace, the argument may be an agent alias from
settings. Without an argument, the current rig's configured default agent is
used. You can also pass a raw command string to probe an ACP adapter directly.

Examples:
  gt acp verify
  gt acp verify opencode
  gt acp verify "./bin/mock-agent-acp"
  gt acp verify --prompt "health check"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runACPVerify,
}

func init() {
	acpVerifyCmd.Flags().BoolVar(&acpVerifyJSON, "json", false, "Output as JSON")
	acpVerifyCmd.Flags().DurationVar(&acpVerifyTimeout, "timeout", 15*time.Second, "Verification timeout")
	acpVerifyCmd.Flags().StringVar(&acpVerifyPrompt, "prompt", "", "Optional session/prompt payload to verify")

	acpCmd.AddCommand(acpVerifyCmd)
	rootCmd.AddCommand(acpCmd)
}

func runACPVerify(cmd *cobra.Command, args []string) error {
	command, argv, cwd, detail, err := resolveACPVerifyTarget(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), acpVerifyTimeout)
	defer cancel()

	result, err := acp.VerifyRuntime(ctx, acp.VerifyOptions{
		Command:    command,
		Args:       argv,
		WorkingDir: cwd,
		Prompt:     acpVerifyPrompt,
	})
	if err != nil && result == nil {
		return err
	}
	if result == nil {
		return err
	}

	if acpVerifyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	renderACPVerifyText(result, detail)
	return err
}

func renderACPVerifyText(result *acp.VerifyResult, detail string) {
	fmt.Printf("%s ACP verify\n", style.Bold.Render("✓"))
	if detail != "" {
		fmt.Printf("  Target: %s\n", detail)
	}
	fmt.Printf("  Transport: %s\n", result.Transport)
	fmt.Printf("  Command: %s %s\n", result.Command, strings.Join(result.Args, " "))
	if result.WorkingDir != "" {
		fmt.Printf("  Working dir: %s\n", result.WorkingDir)
	}
	fmt.Printf("  Protocol version: %d\n", result.ProtocolVersion)
	if result.SessionID != "" {
		fmt.Printf("  Session ID: %s\n", result.SessionID)
	}
	if len(result.Steps) > 0 {
		fmt.Println()
		fmt.Println(style.Bold.Render("Steps"))
		for _, step := range result.Steps {
			status := style.Success.Render("pass")
			if !step.Passed {
				status = style.Warning.Render("fail")
			}
			fmt.Printf("  %-14s %s (%s)\n", step.Method, status, step.Duration.Round(time.Millisecond))
			if step.Detail != "" && step.Detail != "ok" {
				fmt.Printf("    %s\n", step.Detail)
			}
		}
	}
	if result.Stderr != "" {
		fmt.Println()
		fmt.Println(style.Bold.Render("Stderr"))
		fmt.Printf("  %s\n", strings.ReplaceAll(result.Stderr, "\n", "\n  "))
	}
}

func resolveACPVerifyTarget(args []string) (command string, argv []string, cwd string, detail string, err error) {
	var rawTarget string
	if len(args) > 0 {
		rawTarget = strings.TrimSpace(args[0])
	}

	townRoot, _ := workspace.FindFromCwd()
	cwd, _ = os.Getwd()
	rigPath := detectCurrentRigPath(townRoot, cwd)

	if townRoot != "" {
		var rc *config.RuntimeConfig
		var resolved string
		if rawTarget != "" {
			rc, resolved, err = config.ResolveAgentConfigWithOverride(townRoot, rigPath, rawTarget)
		} else {
			rc = config.ResolveAgentConfig(townRoot, rigPath)
			resolved = rc.ResolvedAgent
		}
		if err == nil && rc != nil && config.RuntimeConfigSupportsACP(rc) {
			command, argv = buildACPVerifyCommand(rc)
			if command != "" {
				if strings.TrimSpace(cwd) == "" {
					cwd = townRoot
				}
				if resolved == "" {
					resolved = rawTarget
				}
				detail = strings.TrimSpace(resolved)
				if detail == "" {
					detail = "configured default agent"
				}
				return command, argv, cwd, detail, nil
			}
		}
	}

	if rawTarget == "" {
		return "", nil, "", "", fmt.Errorf("no ACP-capable configured agent found; pass an agent alias or command")
	}

	parts := strings.Fields(rawTarget)
	if len(parts) == 0 {
		return "", nil, "", "", fmt.Errorf("empty ACP target")
	}
	return parts[0], parts[1:], cwd, rawTarget, nil
}

func buildACPVerifyCommand(rc *config.RuntimeConfig) (string, []string) {
	if rc == nil {
		return "", nil
	}
	command := strings.TrimSpace(rc.Command)
	if command == "" {
		return "", nil
	}
	args := append([]string{}, rc.Args...)
	acpCfg := config.GetACPConfigFromRuntime(rc)
	if acpCfg == nil {
		return command, args
	}

	mode := strings.TrimSpace(acpCfg.Mode)
	switch mode {
	case "", config.ACPModeSubcommand:
		if strings.TrimSpace(acpCfg.Command) != "" {
			args = append(args, acpCfg.Command)
		}
		args = append(args, acpCfg.Args...)
	case config.ACPModeFlag:
		args = append(args, acpCfg.Args...)
	case config.ACPModeNative:
		// Native mode uses the binary directly.
	default:
		args = append(args, acpCfg.Args...)
	}
	return command, args
}

func detectCurrentRigPath(townRoot, cwd string) string {
	if townRoot == "" || cwd == "" {
		return ""
	}
	rel, err := filepath.Rel(townRoot, cwd)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	first := strings.Split(rel, string(os.PathSeparator))[0]
	if first == "" || first == "." {
		return ""
	}
	rigPath := filepath.Join(townRoot, first)
	if info, err := os.Stat(filepath.Join(rigPath, "mayor")); err == nil && info.IsDir() {
		return rigPath
	}
	return ""
}
