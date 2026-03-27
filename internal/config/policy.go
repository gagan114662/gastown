package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// MergeRolePolicy overlays override onto base.
// Slice fields replace when override provides values; scalar fields override when set.
func MergeRolePolicy(base, override *RolePolicyConfig) *RolePolicyConfig {
	if base == nil && override == nil {
		return nil
	}
	out := &RolePolicyConfig{}
	if base != nil {
		*out = cloneRolePolicy(base)
	}
	if override == nil {
		return out
	}
	if len(override.Commands) > 0 {
		out.Commands = cloneStrings(override.Commands)
	}
	if len(override.GTSubcommands) > 0 {
		out.GTSubcommands = cloneStrings(override.GTSubcommands)
	}
	if len(override.BDSubcommands) > 0 {
		out.BDSubcommands = cloneStrings(override.BDSubcommands)
	}
	if len(override.WriteRoots) > 0 {
		out.WriteRoots = cloneStrings(override.WriteRoots)
	}
	if len(override.BranchPatterns) > 0 {
		out.BranchPatterns = cloneStrings(override.BranchPatterns)
	}
	if strings.TrimSpace(override.Network) != "" {
		out.Network = strings.TrimSpace(override.Network)
	}
	if override.Push != nil {
		push := *override.Push
		out.Push = &push
	}
	return out
}

// ResolveRolePolicy loads the effective policy for a role from town and optional rig settings.
func ResolveRolePolicy(townRoot, rigPath, role string) *RolePolicyConfig {
	role = ExtractSimpleRole(strings.TrimSpace(role))
	if role == "" {
		return nil
	}

	var merged *RolePolicyConfig
	if townRoot != "" {
		if townSettings, err := LoadOrCreateTownSettings(TownSettingsPath(townRoot)); err == nil && townSettings != nil {
			merged = MergeRolePolicy(merged, lookupRolePolicy(townSettings.Policy, role))
		}
	}

	if rigPath != "" {
		if rigSettings, err := LoadRigSettings(RigSettingsPath(rigPath)); err == nil && rigSettings != nil {
			merged = MergeRolePolicy(merged, lookupRolePolicy(rigSettings.Policy, role))
		}
	}

	if merged == nil {
		return nil
	}
	normalizeRolePolicy(merged, townRoot, rigPath)
	return merged
}

// PolicyEnvValue returns a compact JSON encoding for startup env propagation.
func PolicyEnvValue(policy *RolePolicyConfig) string {
	if policy == nil {
		return ""
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return ""
	}
	return string(data)
}

// RoleAllowsCommand reports whether the policy allows a top-level binary.
// Empty command lists default to allow-all.
func RoleAllowsCommand(policy *RolePolicyConfig, command string) bool {
	if policy == nil || len(policy.Commands) == 0 {
		return true
	}
	command = strings.TrimSpace(command)
	for _, allowed := range policy.Commands {
		if strings.EqualFold(strings.TrimSpace(allowed), command) {
			return true
		}
	}
	return false
}

// RoleAllowsSubcommand reports whether a gt/bd subcommand is allowed.
// Empty subcommand lists default to the caller's built-in allowlist.
func RoleAllowsSubcommand(policy *RolePolicyConfig, binary, subcommand string) bool {
	if policy == nil {
		return true
	}
	subcommand = strings.TrimSpace(subcommand)
	if subcommand == "" {
		return false
	}
	var allowed []string
	switch strings.TrimSpace(binary) {
	case "gt":
		allowed = policy.GTSubcommands
	case "bd":
		allowed = policy.BDSubcommands
	default:
		return true
	}
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), subcommand) {
			return true
		}
	}
	return false
}

// RoleAllowsBranch reports whether a push target branch matches the configured branch policy.
// Empty pattern lists default to allow-all.
func RoleAllowsBranch(policy *RolePolicyConfig, branch string) bool {
	if policy == nil || len(policy.BranchPatterns) == 0 {
		return true
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false
	}
	for _, pattern := range policy.BranchPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, branch); ok {
			return true
		}
		if strings.EqualFold(pattern, branch) {
			return true
		}
	}
	return false
}

func lookupRolePolicy(policy *PolicyConfig, role string) *RolePolicyConfig {
	if policy == nil || len(policy.RolePolicies) == 0 {
		return nil
	}
	if rp, ok := policy.RolePolicies[role]; ok {
		return rp
	}
	for key, rp := range policy.RolePolicies {
		if ExtractSimpleRole(key) == role {
			return rp
		}
	}
	return nil
}

func normalizeRolePolicy(policy *RolePolicyConfig, townRoot, rigPath string) {
	if policy == nil {
		return
	}
	policy.Commands = dedupeNormalized(policy.Commands)
	policy.GTSubcommands = dedupeNormalized(policy.GTSubcommands)
	policy.BDSubcommands = dedupeNormalized(policy.BDSubcommands)
	policy.BranchPatterns = dedupeNormalized(policy.BranchPatterns)
	for i, root := range policy.WriteRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		root = strings.ReplaceAll(root, "$GT_ROOT", townRoot)
		root = strings.ReplaceAll(root, "$GT_RIG_ROOT", rigPath)
		policy.WriteRoots[i] = filepath.Clean(root)
	}
	policy.WriteRoots = dedupeNormalized(policy.WriteRoots)
	policy.Network = strings.TrimSpace(policy.Network)
}

func cloneRolePolicy(src *RolePolicyConfig) RolePolicyConfig {
	if src == nil {
		return RolePolicyConfig{}
	}
	out := RolePolicyConfig{
		Commands:       cloneStrings(src.Commands),
		GTSubcommands:  cloneStrings(src.GTSubcommands),
		BDSubcommands:  cloneStrings(src.BDSubcommands),
		WriteRoots:     cloneStrings(src.WriteRoots),
		BranchPatterns: cloneStrings(src.BranchPatterns),
		Network:        src.Network,
	}
	if src.Push != nil {
		push := *src.Push
		out.Push = &push
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func dedupeNormalized(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

// ResolvePolicyContext returns the town root, rig root, and normalized role derived from cwd/env.
func ResolvePolicyContext(cwd, envRole, envRig, envTownRoot string) (townRoot, rigPath, role string) {
	role = ExtractSimpleRole(strings.TrimSpace(envRole))
	townRoot = strings.TrimSpace(envTownRoot)
	if townRoot == "" && cwd != "" {
		if statTownRoot, err := findTownRootFromPath(cwd); err == nil {
			townRoot = statTownRoot
		}
	}
	rig := strings.TrimSpace(envRig)
	if rig != "" && townRoot != "" {
		rigPath = filepath.Join(townRoot, rig)
		if info, err := os.Stat(rigPath); err != nil || !info.IsDir() {
			rigPath = ""
		}
	}
	return townRoot, rigPath, role
}

func findTownRootFromPath(start string) (string, error) {
	absDir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	const marker = "mayor/town.json"
	current := absDir
	for {
		if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
