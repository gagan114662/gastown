package ctxstack

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func requireSQLite3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}
}

func TestStoreRoundTripAndSearch(t *testing.T) {
	requireSQLite3(t)

	townRoot := t.TempDir()
	store, err := Open(townRoot)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	summary := SessionSummary{
		SessionID: "sess-1",
		Role:      "polecat",
		Rig:       "rig-1",
		Agent:     "codex",
		WorkBead:  "gt-123",
		Source:    "handoff_cycle",
		Summary:   "Need to finish the API endpoint and re-run verifier.",
		Changes:   "Added handler scaffolding.",
		NextSteps: "Implement database write path.",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.PutSessionSummary(summary); err != nil {
		t.Fatalf("PutSessionSummary: %v", err)
	}

	if err := store.AddScratchpadEntry(ScratchpadEntry{
		SessionID: "sess-1",
		Kind:      "note",
		Text:      "private reasoning only",
	}); err != nil {
		t.Fatalf("AddScratchpadEntry: %v", err)
	}

	if err := store.UpsertRetrievalDoc(RetrievalDoc{
		DocID:     "memory:project:api-endpoint",
		Tier:      TierCold,
		Source:    "memory",
		Rig:       "rig-1",
		Role:      "polecat",
		Bead:      "gt-123",
		Tags:      []string{"memory", "project"},
		Text:      "API endpoint requires transaction retries on conflict.",
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertRetrievalDoc: %v", err)
	}

	summaries, err := store.ListSessionSummaries(SummaryFilter{WorkBead: "gt-123", Limit: 5})
	if err != nil {
		t.Fatalf("ListSessionSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	results, err := store.SearchRetrieval(SearchOptions{
		Query: "API endpoint transaction",
		Rig:   "rig-1",
		Role:  "polecat",
		Bead:  "gt-123",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchRetrieval: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected retrieval hits")
	}
	if results[0].DocID != "memory:project:api-endpoint" && results[0].Source != "session_summary" {
		t.Fatalf("unexpected top result: %+v", results[0])
	}

	// Scratchpad entries must remain private and should not be searchable.
	for _, result := range results {
		if result.Text == "private reasoning only" {
			t.Fatal("scratchpad content leaked into retrieval search")
		}
	}
}

func TestBuildPrimeSnapshotPrefersWarmContext(t *testing.T) {
	requireSQLite3(t)

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Now().UTC()
	if err := store.PutSessionSummary(SessionSummary{
		SessionID: "sess-2",
		Role:      "crew",
		Rig:       "rig-2",
		WorkBead:  "gt-456",
		Source:    "handoff_auto",
		Summary:   "Crew worker already updated the parser. Resume from tests.",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("PutSessionSummary: %v", err)
	}
	if err := store.UpsertRetrievalDoc(RetrievalDoc{
		DocID:     "memory:project:parser",
		Tier:      TierCold,
		Source:    "memory",
		Rig:       "rig-2",
		Role:      "crew",
		Bead:      "gt-456",
		Text:      "Parser work depends on test fixtures.",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertRetrievalDoc: %v", err)
	}

	snapshot, err := store.BuildPrimeSnapshot(PrimeRequest{
		SessionID: "sess-2",
		Role:      "crew",
		Rig:       "rig-2",
		WorkBead:  "gt-456",
		Query:     "parser fixtures",
		MaxItems:  4,
	}, DefaultSettings(), RuntimeCapabilities{})
	if err != nil {
		t.Fatalf("BuildPrimeSnapshot: %v", err)
	}
	if snapshot.PrimarySummary == nil {
		t.Fatal("expected primary summary")
	}
	if snapshot.PrimarySummary.Source != "handoff_auto" {
		t.Fatalf("unexpected primary summary source: %s", snapshot.PrimarySummary.Source)
	}
	if len(snapshot.Docs) == 0 {
		t.Fatal("expected retrieval docs")
	}
}

func TestDefaultSettingsAllocate(t *testing.T) {
	allocation := DefaultSettings().Allocate(RuntimeCapabilities{MaxContextTokens: 1000})
	if allocation.MaxTokens != 1000 {
		t.Fatalf("MaxTokens = %d, want 1000", allocation.MaxTokens)
	}
	if allocation.Instructions != 100 || allocation.Retrieved != 200 || allocation.CarryForward != 300 {
		t.Fatalf("unexpected allocation: %+v", allocation)
	}
	if allocation.SafetySlack != 100 {
		t.Fatalf("SafetySlack = %d, want 100", allocation.SafetySlack)
	}
}

func TestInferUsageFromEnvOverride(t *testing.T) {
	t.Setenv("GT_CONTEXT_BUDGET_TOKENS", "500")
	sample, err := InferUsage(t.TempDir(), 1000)
	if err != nil {
		t.Fatalf("InferUsage: %v", err)
	}
	if sample == nil {
		t.Fatal("expected usage sample")
	}
	if sample.UsedTokens != 500 || sample.MaxTokens != 1000 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
}

func TestOpenCreatesContextSQLite(t *testing.T) {
	requireSQLite3(t)
	townRoot := t.TempDir()
	store, err := Open(townRoot)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := os.Stat(filepath.Join(townRoot, ".runtime", dbFileName)); err != nil {
		t.Fatalf("context sqlite missing: %v", err)
	}
	if store.Path() == "" {
		t.Fatal("expected store path")
	}
}
