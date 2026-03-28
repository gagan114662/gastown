package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	policyRole   string
	policyRig    string
	policyJSON   bool
	policyBranch string
)

var policyCmd = &cobra.Command{
	Use:     "policy",
	GroupID: GroupConfig,
	Short:   "Inspect role governance policy",
	RunE:    requireSubcommand,
}

var policyShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the effective role policy for the current context",
	RunE:  runPolicyShow,
}

var policyPrePushCheckCmd = &cobra.Command{
	Use:    "pre-push-check",
	Hidden: true,
	Short:  "Enforce branch/push policy for git hooks",
	RunE:   runPolicyPrePushCheck,
}

func init() {
	policyShowCmd.Flags().StringVar(&policyRole, "role", "", "Role to resolve (defaults to GT_ROLE)")
	policyShowCmd.Flags().StringVar(&policyRig, "rig", "", "Rig name override")
	policyShowCmd.Flags().BoolVar(&policyJSON, "json", false, "Output JSON")

	policyPrePushCheckCmd.Flags().StringVar(&policyRole, "role", "", "Role to resolve (defaults to GT_ROLE)")
	policyPrePushCheckCmd.Flags().StringVar(&policyRig, "rig", "", "Rig name override")
	policyPrePushCheckCmd.Flags().StringVar(&policyBranch, "branch", "", "Target branch name")

	policyCmd.AddCommand(policyShowCmd)
	policyCmd.AddCommand(policyPrePushCheckCmd)
	rootCmd.AddCommand(policyCmd)
}

func runPolicyShow(cmd *cobra.Command, args []string) error {
	policy, role, townRoot, rigPath, err := resolveRolePolicyContext(policyRole, policyRig, false)
	if err != nil {
		return err
	}
	if policyJSON {
		payload := map[string]any{
			"role":      role,
			"town_root": townRoot,
			"rig_path":  rigPath,
			"policy":    policy,
			"proxy": map[string][]string{
				"gt": filteredGTSubcommands(policy),
				"bd": filteredBDSubcommands(policy),
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	fmt.Printf("%s Role policy\n", style.Bold.Render("✓"))
	fmt.Printf("  Role: %s\n", role)
	if townRoot != "" {
		fmt.Printf("  Town: %s\n", townRoot)
	}
	if rigPath != "" {
		fmt.Printf("  Rig:  %s\n", rigPath)
	}
	if policy == nil {
		fmt.Printf("  %s no explicit policy configured\n", style.Dim.Render("·"))
		return nil
	}
	fmt.Printf("  Commands: %s\n", formatOrAny(policy.Commands))
	fmt.Printf("  gt: %s\n", formatOrAny(filteredGTSubcommands(policy)))
	fmt.Printf("  bd: %s\n", formatOrAny(filteredBDSubcommands(policy)))
	fmt.Printf("  Write roots: %s\n", formatOrAny(policy.WriteRoots))
	fmt.Printf("  Branches: %s\n", formatOrAny(policy.BranchPatterns))
	if strings.TrimSpace(policy.Network) == "" {
		fmt.Printf("  Network: any\n")
	} else {
		fmt.Printf("  Network: %s\n", policy.Network)
	}
	if policy.Push == nil {
		fmt.Printf("  Push: any\n")
	} else {
		fmt.Printf("  Push: %t\n", *policy.Push)
	}
	return nil
}

func runPolicyPrePushCheck(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(policyBranch) == "" {
		return fmt.Errorf("--branch is required")
	}
	policy, role, _, _, err := resolveRolePolicyContext(policyRole, policyRig, true)
	if err != nil {
		return err
	}
	if policy == nil {
		return nil
	}
	if policy.Push != nil && !*policy.Push {
		return fmt.Errorf("push denied by role policy for %s", role)
	}
	if !config.RoleAllowsBranch(policy, policyBranch) {
		return fmt.Errorf("branch %q denied by role policy for %s (allowed: %s)", policyBranch, role, formatOrAny(policy.BranchPatterns))
	}
	return nil
}

func resolveRolePolicyContext(roleOverride, rigOverride string, allowDefaultRole bool) (*config.RolePolicyConfig, string, string, string, error) {
	cwd, _ := os.Getwd()
	envRole := os.Getenv("GT_ROLE")
	if strings.TrimSpace(roleOverride) != "" {
		envRole = roleOverride
	}
	envRig := os.Getenv("GT_RIG")
	if strings.TrimSpace(rigOverride) != "" {
		envRig = rigOverride
	}
	townRoot, rigPath, role := config.ResolvePolicyContext(cwd, envRole, envRig, os.Getenv("GT_ROOT"))
	if role == "" && allowDefaultRole {
		role = "polecat"
	}
	if role == "" {
		return nil, "", townRoot, rigPath, fmt.Errorf("no role available; pass --role or run inside an agent session")
	}
	if rigPath == "" && envRig != "" && townRoot != "" {
		rigPath = filepath.Join(townRoot, envRig)
	}
	return config.ResolveRolePolicy(townRoot, rigPath, role), role, townRoot, rigPath, nil
}

func filteredGTSubcommands(policy *config.RolePolicyConfig) []string {
	return filterSubcommands("gt", discoverAnnotatedSubcommands(), policy)
}

func filteredBDSubcommands(policy *config.RolePolicyConfig) []string {
	return filterSubcommands("bd", strings.Split(bdSafeSubcmds, ","), policy)
}

func discoverAnnotatedSubcommands() []string {
	var gtSubs []string
	for _, c := range rootCmd.Commands() {
		if c.Annotations[AnnotationPolecatSafe] == "true" {
			gtSubs = append(gtSubs, c.Name())
		}
	}
	sort.Strings(gtSubs)
	return gtSubs
}

func filterSubcommands(binary string, defaults []string, policy *config.RolePolicyConfig) []string {
	if !config.RoleAllowsCommand(policy, binary) {
		return nil
	}
	var filtered []string
	for _, sub := range defaults {
		if config.RoleAllowsSubcommand(policy, binary, sub) {
			filtered = append(filtered, sub)
		}
	}
	return filtered
}

func formatOrAny(values []string) string {
	if len(values) == 0 {
		return "any"
	}
	return strings.Join(values, ", ")
}
