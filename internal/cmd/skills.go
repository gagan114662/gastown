package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	gtSkills "github.com/steveyegge/gastown/internal/skills"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var skillsJSON bool

var skillsCmd = &cobra.Command{
	Use:     "skills",
	GroupID: GroupConfig,
	Short:   "Manage canonical skill packages",
	RunE:    requireSubcommand,
}

var skillsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync canonical skill packages into .claude/skills",
	RunE:  runSkillsSync,
}

var skillsTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Validate skill packages and fixtures",
	RunE:  runSkillsTest,
}

var skillsAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit skill packages for legacy format or sync drift",
	RunE:  runSkillsAudit,
}

func init() {
	skillsSyncCmd.Flags().BoolVar(&skillsJSON, "json", false, "Output as JSON")
	skillsTestCmd.Flags().BoolVar(&skillsJSON, "json", false, "Output as JSON")
	skillsAuditCmd.Flags().BoolVar(&skillsJSON, "json", false, "Output as JSON")

	skillsCmd.AddCommand(skillsSyncCmd)
	skillsCmd.AddCommand(skillsTestCmd)
	skillsCmd.AddCommand(skillsAuditCmd)
	rootCmd.AddCommand(skillsCmd)
}

func runSkillsSync(cmd *cobra.Command, args []string) error {
	root, runtimeDir, err := resolveSkillPaths()
	if err != nil {
		return err
	}
	result, err := gtSkills.Sync(root, runtimeDir)
	if err != nil {
		return err
	}
	if skillsJSON {
		return outputSkillsJSON(result)
	}
	fmt.Printf("%s Synced skills into %s\n", style.Success.Render("✓"), runtimeDir)
	for _, name := range result.Copied {
		fmt.Printf("  %s %s\n", style.Success.Render("↑"), name)
	}
	for _, name := range result.Skipped {
		fmt.Printf("  %s %s already current\n", style.Dim.Render("·"), name)
	}
	return nil
}

func runSkillsTest(cmd *cobra.Command, args []string) error {
	root, _, err := resolveSkillPaths()
	if err != nil {
		return err
	}
	results, err := gtSkills.Test(root)
	if err != nil {
		return err
	}
	if skillsJSON {
		return outputSkillsJSON(results)
	}
	return printSkillResults("Skill test", results)
}

func runSkillsAudit(cmd *cobra.Command, args []string) error {
	root, runtimeDir, err := resolveSkillPaths()
	if err != nil {
		return err
	}
	results, err := gtSkills.Audit(root, runtimeDir)
	if err != nil {
		return err
	}
	if skillsJSON {
		return outputSkillsJSON(results)
	}
	return printSkillResults("Skill audit", results)
}

func resolveSkillPaths() (root, runtimeDir string, err error) {
	townRoot, _ := workspace.FindFromCwd()
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	root = filepath.Join(cwd, "skills")
	if townRoot != "" {
		root = filepath.Join(townRoot, "skills")
	}
	runtimeDir = filepath.Join(filepath.Dir(root), ".claude", "skills")
	if townRoot != "" {
		runtimeDir = filepath.Join(townRoot, ".claude", "skills")
	}
	return root, runtimeDir, nil
}

func outputSkillsJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printSkillResults(title string, results []gtSkills.TestResult) error {
	if len(results) == 0 {
		fmt.Printf("%s %s: no packages found\n", style.Dim.Render("·"), title)
		return nil
	}
	failing := 0
	for _, result := range results {
		if !result.Passed {
			failing++
		}
	}
	fmt.Printf("%s (%d packages)\n\n", style.Bold.Render(title), len(results))
	for _, result := range results {
		icon := style.Success.Render("✓")
		if !result.Passed {
			icon = style.Warning.Render("!")
		}
		fmt.Printf("  %s %s\n", icon, result.Package)
		for _, issue := range result.Issues {
			fmt.Printf("    %s\n", issue)
		}
	}
	if failing > 0 {
		return fmt.Errorf("%d package(s) need attention", failing)
	}
	return nil
}
