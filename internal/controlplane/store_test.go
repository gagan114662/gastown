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
