package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Package describes a skill package rooted at skills/<name>.
type Package struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Path        string   `json:"path"`
	HasMetadata bool     `json:"has_metadata"`
	HasFixtures bool     `json:"has_fixtures"`
	Providers   []string `json:"providers,omitempty"`
}

type metadataFile struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Version     string   `toml:"version"`
	Providers   []string `toml:"providers"`
}

type fixture struct {
	Query  string   `json:"query"`
	Expect []string `json:"expect"`
}

type TestResult struct {
	Package string   `json:"package"`
	Passed  bool     `json:"passed"`
	Issues  []string `json:"issues,omitempty"`
}

type SyncResult struct {
	Copied  []string `json:"copied,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
}

// Discover scans a skills root and returns available packages.
func Discover(root string) ([]Package, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var packages []Package
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		pkg, err := Load(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		if pkg.Name != "" {
			packages = append(packages, *pkg)
		}
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
	})
	return packages, nil
}

// Load loads one skill package directory.
func Load(dir string) (*Package, error) {
	skillPath := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		return nil, nil
	}
	pkg := &Package{
		Name: filepath.Base(dir),
		Path: dir,
	}
	if metaPath := filepath.Join(dir, "skill.toml"); fileExists(metaPath) {
		var meta metadataFile
		if _, err := toml.DecodeFile(metaPath, &meta); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", metaPath, err)
		}
		if strings.TrimSpace(meta.Name) != "" {
			pkg.Name = strings.TrimSpace(meta.Name)
		}
		pkg.Description = strings.TrimSpace(meta.Description)
		pkg.Providers = append([]string(nil), meta.Providers...)
		pkg.HasMetadata = true
	} else if name, desc := parseSkillFrontmatter(skillPath); name != "" || desc != "" {
		if name != "" {
			pkg.Name = name
		}
		pkg.Description = desc
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "fixtures")); err == nil && len(entries) > 0 {
		pkg.HasFixtures = true
	}
	return pkg, nil
}

// Test validates packages and optional fixtures.
func Test(root string) ([]TestResult, error) {
	packages, err := Discover(root)
	if err != nil {
		return nil, err
	}
	results := make([]TestResult, 0, len(packages))
	for _, pkg := range packages {
		res := TestResult{Package: pkg.Name, Passed: true}
		if !pkg.HasMetadata {
			res.Issues = append(res.Issues, "missing skill.toml")
		}
		if issues := validateFixtures(pkg.Path); len(issues) > 0 {
			res.Issues = append(res.Issues, issues...)
		}
		if len(res.Issues) > 0 {
			res.Passed = false
		}
		results = append(results, res)
	}
	return results, nil
}

// Audit compares canonical skill packages with their synced runtime copies.
func Audit(root, runtimeDir string) ([]TestResult, error) {
	packages, err := Discover(root)
	if err != nil {
		return nil, err
	}
	var results []TestResult
	for _, pkg := range packages {
		res := TestResult{Package: pkg.Name, Passed: true}
		targetDir := filepath.Join(runtimeDir, pkg.Name)
		if !fileExists(filepath.Join(pkg.Path, "skill.toml")) {
			res.Issues = append(res.Issues, "legacy package format (no skill.toml)")
		}
		if !fileExists(filepath.Join(targetDir, "SKILL.md")) {
			res.Issues = append(res.Issues, "not synced to runtime")
		} else if same, err := dirsMatch(pkg.Path, targetDir); err != nil {
			res.Issues = append(res.Issues, err.Error())
		} else if !same {
			res.Issues = append(res.Issues, "runtime copy drifted from canonical package")
		}
		if len(res.Issues) > 0 {
			res.Passed = false
		}
		results = append(results, res)
	}
	return results, nil
}

// Sync copies packages to a runtime-specific skills directory.
func Sync(root, runtimeDir string) (*SyncResult, error) {
	packages, err := Discover(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return nil, err
	}
	result := &SyncResult{}
	for _, pkg := range packages {
		target := filepath.Join(runtimeDir, pkg.Name)
		same, err := dirsMatch(pkg.Path, target)
		if err != nil {
			return nil, err
		}
		if same {
			result.Skipped = append(result.Skipped, pkg.Name)
			continue
		}
		if err := copyDir(pkg.Path, target); err != nil {
			return nil, err
		}
		result.Copied = append(result.Copied, pkg.Name)
	}
	return result, nil
}

func parseSkillFrontmatter(skillPath string) (name, description string) {
	data, err := os.ReadFile(skillPath) //nolint:gosec // local package file
	if err != nil {
		return "", ""
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return "", ""
	}
	parts := strings.SplitN(text[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return "", ""
	}
	for _, line := range strings.Split(parts[0], "\n") {
		if after, ok := strings.CutPrefix(line, "name:"); ok {
			name = strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "description:"); ok {
			description = strings.TrimSpace(after)
		}
	}
	return name, description
}

func validateFixtures(dir string) []string {
	var issues []string
	fixturesDir := filepath.Join(dir, "fixtures")
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		return nil
	}
	skillData, err := os.ReadFile(filepath.Join(dir, "SKILL.md")) //nolint:gosec // local package file
	if err != nil {
		return []string{err.Error()}
	}
	skillText := strings.ToLower(string(skillData))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fixturesDir, entry.Name())) //nolint:gosec // local package file
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s unreadable", entry.Name()))
			continue
		}
		var fx fixture
		if err := json.Unmarshal(data, &fx); err != nil {
			issues = append(issues, fmt.Sprintf("%s invalid JSON", entry.Name()))
			continue
		}
		if strings.TrimSpace(fx.Query) == "" {
			issues = append(issues, fmt.Sprintf("%s missing query", entry.Name()))
		}
		for _, want := range fx.Expect {
			if !strings.Contains(skillText, strings.ToLower(strings.TrimSpace(want))) {
				issues = append(issues, fmt.Sprintf("%s missing expected phrase %q", entry.Name(), want))
			}
		}
	}
	return issues
}

func dirsMatch(src, dst string) (bool, error) {
	if !fileExists(dst) {
		return false, nil
	}
	srcHash, err := dirHash(src)
	if err != nil {
		return false, err
	}
	dstHash, err := dirHash(dst)
	if err != nil {
		return false, err
	}
	return srcHash == dstHash, nil
}

func dirHash(dir string) (string, error) {
	entries := []string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // local package file
		if err != nil {
			return err
		}
		entries = append(entries, rel+"::"+string(data))
		return nil
	})
	if err != nil {
		return "", err
	}
	return strings.Join(entries, "\n"), nil
}

func copyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // local package file
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode) //nolint:gosec // local package file
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
