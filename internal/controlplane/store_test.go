package controlplane

import (
	"os/exec"
	"testing"
	"time"
)

func TestStoreRecordEventAndIncident(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	event := TownEvent{
		EventID:    "evt-1",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Kind:       "session_death",
		Type:       "session_death",
		Actor:      "daemon",
		Session:    "gt-test-agent",
		Outcome:    "error",
		Reason:     "zombie cleanup",
		Visibility: "both",
		Source:     "gt",
		Payload: map[string]interface{}{
			"caller": "daemon",
		},
	}
	if err := store.RecordEvent(event); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	events, err := store.ListEvents(10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ListEvents len = %d, want 1", len(events))
	}
	if events[0].EventID != event.EventID {
		t.Fatalf("EventID = %q, want %q", events[0].EventID, event.EventID)
	}
	if events[0].Reason != event.Reason {
		t.Fatalf("Reason = %q, want %q", events[0].Reason, event.Reason)
	}

	incidents, err := store.ListIncidents(10)
	if err != nil {
		t.Fatalf("ListIncidents: %v", err)
	}
	if len(incidents) != 1 {
		t.Fatalf("ListIncidents len = %d, want 1", len(incidents))
	}
	if incidents[0].Kind != "session_death" {
		t.Fatalf("Incident kind = %q, want session_death", incidents[0].Kind)
	}
}

func TestStoreUpsertAgentRuntime(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	record := AgentRuntimeRecord{
		AgentID:         "hq-mayor",
		Role:            "mayor",
		Session:         "hq-mayor",
		Status:          "running",
		StatusReason:    "session started",
		SourceAgreement: "legacy-shadow",
		LastEventID:     "evt-1",
		LastEventKind:   "session_start",
		LastEventTS:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.UpsertAgentRuntime(record); err != nil {
		t.Fatalf("UpsertAgentRuntime: %v", err)
	}

	got, err := store.GetAgentRuntime("hq-mayor")
	if err != nil {
		t.Fatalf("GetAgentRuntime: %v", err)
	}
	if got == nil {
		t.Fatal("GetAgentRuntime returned nil")
	}
	if got.Status != "running" {
		t.Fatalf("Status = %q, want running", got.Status)
	}
	if got.LastEventKind != "session_start" {
		t.Fatalf("LastEventKind = %q, want session_start", got.LastEventKind)
	}
}

func TestStoreSupervisorState(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}

	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	lease, err := store.AcquireLease(LeaseRecord{
		LeaseID: LeaseKey("refinery", "gastown"),
		Service: "refinery",
		Rig:     "gastown",
		Session: "gt-gastown-refinery",
		Holder:  "refinery",
		Status:  "active",
		Detail:  "tmux",
	})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease == nil || lease.Status != "active" {
		t.Fatalf("lease = %#v, want active lease", lease)
	}

	if err := store.UpsertRespawnCounter(RespawnCounter{
		BeadID:      "gt-abc",
		Rig:         "gastown",
		Count:       3,
		MaxCount:    4,
		LastRespawn: time.Now().UTC().Format(time.RFC3339),
		Blocked:     false,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertRespawnCounter: %v", err)
	}
	respawn, err := store.GetRespawnCounter("gt-abc")
	if err != nil {
		t.Fatalf("GetRespawnCounter: %v", err)
	}
	if respawn == nil || respawn.Count != 3 {
		t.Fatalf("respawn = %#v, want count 3", respawn)
	}

	if err := store.UpsertRedispatchRecord(RedispatchRecord{
		BeadID:          "gt-abc",
		SourceRig:       "gastown",
		TargetRig:       "gastown",
		AttemptCount:    2,
		LastAttemptTime: time.Now().UTC().Format(time.RFC3339),
		LastAction:      "redispatched",
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertRedispatchRecord: %v", err)
	}
	redispatch, err := store.GetRedispatchRecord("gt-abc")
	if err != nil {
		t.Fatalf("GetRedispatchRecord: %v", err)
	}
	if redispatch == nil || redispatch.AttemptCount != 2 {
		t.Fatalf("redispatch = %#v, want attempt count 2", redispatch)
	}

	if err := store.UpsertCleanupState(CleanupState{
		CleanupID:    CleanupKey("gastown", "alpha"),
		Rig:          "gastown",
		PolecatName:  "alpha",
		BeadID:       "gt-abc",
		Session:      "gt-gastown-p-alpha",
		Status:       "cleanup-tracked",
		Blocker:      "has_uncommitted",
		AttemptCount: 1,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertCleanupState: %v", err)
	}
	cleanup, err := store.GetCleanupStateByPolecat("gastown", "alpha")
	if err != nil {
		t.Fatalf("GetCleanupStateByPolecat: %v", err)
	}
	if cleanup == nil || cleanup.Status != "cleanup-tracked" {
		t.Fatalf("cleanup = %#v, want cleanup-tracked", cleanup)
	}

	if err := store.RecordDependencyHealth(DependencyHealth{
		DependencyKey: DependencyKey("dolt", "gastown"),
		Name:          "dolt",
		Scope:         "gastown",
		Status:        "degraded",
		Detail:        "connection refused",
		CheckedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("RecordDependencyHealth: %v", err)
	}
	deps, err := store.ListDependencyHealth()
	if err != nil {
		t.Fatalf("ListDependencyHealth: %v", err)
	}
	if len(deps) != 1 || deps[0].Status != "degraded" {
		t.Fatalf("deps = %#v, want one degraded dependency", deps)
	}

	if err := store.ReleaseLease(LeaseKey("refinery", "gastown"), "stopped"); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	released, err := store.GetLease(LeaseKey("refinery", "gastown"))
	if err != nil {
		t.Fatalf("GetLease after release: %v", err)
	}
	if released == nil || released.Status != "released" {
		t.Fatalf("released lease = %#v, want released", released)
	}
}
