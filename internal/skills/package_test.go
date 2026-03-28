package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAndTest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "deploy")
	if err := os.MkdirAll(filepath.Join(dir, "fixtures"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skill.toml"), []byte("name = \"deploy\"\ndescription = \"Deploy skill\"\nproviders = [\"claude\"]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# deploy\nRun deploy checks and verify rollout.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixtures", "smoke.json"), []byte(`{"query":"deploy","expect":["verify rollout"]}`), 0644); err != nil {
		t.Fatal(err)
	}

	packages, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(packages) != 1 || packages[0].Name != "deploy" {
		t.Fatalf("unexpected packages: %+v", packages)
	}

	results, err := Test(root)
	if err != nil {
		t.Fatalf("Test() error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected passing result, got %+v", results)
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	packages, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(packages) != 0 {
		t.Fatalf("expected no packages, got %+v", packages)
	}
}

func TestSyncAndAudit(t *testing.T) {
	root := t.TempDir()
	runtimeDir := t.TempDir()
	dir := filepath.Join(root, "agent-ci")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: agent-ci\ndescription: CI helper\n---\n# agent-ci\n"), 0644); err != nil {
		t.Fatal(err)
	}

	syncResult, err := Sync(root, runtimeDir)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}
	if len(syncResult.Copied) != 1 || syncResult.Copied[0] != "agent-ci" {
		t.Fatalf("unexpected sync result: %+v", syncResult)
	}

	auditResults, err := Audit(root, runtimeDir)
	if err != nil {
		t.Fatalf("Audit() error: %v", err)
	}
	if len(auditResults) != 1 || auditResults[0].Passed {
		t.Fatalf("expected audit findings for legacy package, got %+v", auditResults)
	}
}
