package config

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMergeRolePolicy(t *testing.T) {
	allowPush := true
	denyPush := false
	base := &RolePolicyConfig{
		Commands:       []string{"gt", "bd"},
		GTSubcommands:  []string{"prime", "done"},
		BDSubcommands:  []string{"show"},
		WriteRoots:     []string{"$GT_RIG_ROOT/polecats"},
		BranchPatterns: []string{"polecat/*"},
		Network:        "loopback",
		Push:           &allowPush,
	}
	override := &RolePolicyConfig{
		GTSubcommands:  []string{"prime", "handoff"},
		BranchPatterns: []string{"integration/*"},
		Push:           &denyPush,
	}

	got := MergeRolePolicy(base, override)
	if !reflect.DeepEqual(got.Commands, []string{"gt", "bd"}) {
		t.Fatalf("Commands = %v", got.Commands)
	}
	if !reflect.DeepEqual(got.GTSubcommands, []string{"prime", "handoff"}) {
		t.Fatalf("GTSubcommands = %v", got.GTSubcommands)
	}
	if !reflect.DeepEqual(got.BranchPatterns, []string{"integration/*"}) {
		t.Fatalf("BranchPatterns = %v", got.BranchPatterns)
	}
	if got.Push == nil || *got.Push {
		t.Fatalf("Push = %v, want false", got.Push)
	}
}

func TestResolveRolePolicy(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "gastown")
	if err := SaveTownConfig(filepath.Join(townRoot, "mayor", "town.json"), &TownConfig{
		Type:      "town",
		Version:   1,
		Name:      "test-town",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("SaveTownConfig: %v", err)
	}
	townSettings := NewTownSettings()
	townSettings.Policy = &PolicyConfig{
		RolePolicies: map[string]*RolePolicyConfig{
			"polecat": {
				Commands:       []string{"gt", "bd"},
				GTSubcommands:  []string{"prime", "done"},
				BranchPatterns: []string{"polecat/*"},
			},
		},
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	rigSettings := NewRigSettings()
	rigSettings.Policy = &PolicyConfig{
		RolePolicies: map[string]*RolePolicyConfig{
			"polecat": {
				GTSubcommands: []string{"prime", "handoff"},
				WriteRoots:    []string{"$GT_RIG_ROOT/polecats", "$GT_ROOT/.runtime"},
			},
		},
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	got := ResolveRolePolicy(townRoot, rigPath, "gastown/polecats/furiosa")
	if got == nil {
		t.Fatal("ResolveRolePolicy() = nil")
	}
	if !reflect.DeepEqual(got.Commands, []string{"gt", "bd"}) {
		t.Fatalf("Commands = %v", got.Commands)
	}
	if !reflect.DeepEqual(got.GTSubcommands, []string{"prime", "handoff"}) {
		t.Fatalf("GTSubcommands = %v", got.GTSubcommands)
	}
	wantRoots := []string{
		filepath.Join(rigPath, "polecats"),
		filepath.Join(townRoot, ".runtime"),
	}
	if !reflect.DeepEqual(got.WriteRoots, wantRoots) {
		t.Fatalf("WriteRoots = %v, want %v", got.WriteRoots, wantRoots)
	}
}

func TestRoleAllowsBranch(t *testing.T) {
	policy := &RolePolicyConfig{BranchPatterns: []string{"polecat/*", "integration/*"}}
	if !RoleAllowsBranch(policy, "polecat/nux-1") {
		t.Fatal("expected polecat branch to be allowed")
	}
	if RoleAllowsBranch(policy, "main") {
		t.Fatal("expected main to be denied")
	}
}
