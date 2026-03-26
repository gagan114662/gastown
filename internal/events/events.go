// Package events provides event logging for the gt activity feed.
//
// Events are written to ~/gt/.events.jsonl (raw audit log) and later
// curated by the feed daemon into ~/.feed.jsonl (user-facing).
package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/controlplane"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Event represents an activity event in Gas Town.
type Event struct {
	EventID    string                 `json:"event_id,omitempty"`
	Timestamp  string                 `json:"ts"`
	Source     string                 `json:"source"`
	Kind       string                 `json:"kind,omitempty"`
	Type       string                 `json:"type"`
	Actor      string                 `json:"actor"`
	Role       string                 `json:"role,omitempty"`
	Rig        string                 `json:"rig,omitempty"`
	Session    string                 `json:"session,omitempty"`
	RunID      string                 `json:"run_id,omitempty"`
	BeadID     string                 `json:"bead_id,omitempty"`
	MRID       string                 `json:"mr_id,omitempty"`
	ConvoyID   string                 `json:"convoy_id,omitempty"`
	Outcome    string                 `json:"outcome,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	Evidence   map[string]interface{} `json:"evidence,omitempty"`
	Visibility string                 `json:"visibility"`
}

// Visibility levels for events.
const (
	VisibilityAudit = "audit" // Only in raw events log
	VisibilityFeed  = "feed"  // Appears in curated feed
	VisibilityBoth  = "both"  // Both audit and feed
)

// Common event types for gt commands.
const (
	TypeSling   = "sling"
	TypeHook    = "hook"
	TypeUnhook  = "unhook"
	TypeHandoff = "handoff"
	TypeDone    = "done"
	TypeMail    = "mail"
	TypeSpawn   = "spawn"
	TypeKill    = "kill"
	TypeNudge   = "nudge"
	TypeBoot    = "boot"
	TypeHalt    = "halt"

	// Session events (for seance discovery)
	TypeSessionStart = "session_start"
	TypeSessionEnd   = "session_end"

	// Session death events (for crash investigation)
	TypeSessionDeath = "session_death" // Feed-visible session termination
	TypeMassDeath    = "mass_death"    // Multiple sessions died in short window

	// Witness patrol events
	TypePatrolStarted    = "patrol_started"
	TypePolecatChecked   = "polecat_checked"
	TypePolecatNudged    = "polecat_nudged"
	TypeEscalationSent   = "escalation_sent"
	TypeEscalationAcked  = "escalation_acked"
	TypeEscalationClosed = "escalation_closed"
	TypePatrolComplete   = "patrol_complete"

	// Merge queue events (emitted by refinery)
	TypeMergeStarted = "merge_started"
	TypeMerged       = "merged"
	TypeMergeFailed  = "merge_failed"
	TypeMergeSkipped = "merge_skipped"

	// Scheduler events
	TypeSchedulerEnqueue        = "scheduler_enqueue"         // Bead scheduled for deferred dispatch
	TypeSchedulerDispatch       = "scheduler_dispatch"        // Bead dispatched from scheduler
	TypeSchedulerDispatchFailed = "scheduler_dispatch_failed" // Bead dispatch failed (requeued)
	TypeSchedulerCloseRetry     = "scheduler_close_retry"     // Context close needed last-resort attempt

	// Rollback events (emitted by gt mol step fail)
	TypeStepFailed      = "step_failed"      // A formula step was marked as failed
	TypeRollbackStarted = "rollback_started" // Rollback sequence initiated after step failure
	TypeRollbackStep    = "rollback_step"    // Wide event: one compensating action executed during rollback
	TypeRollbackDone    = "rollback_done"    // Rollback sequence completed (all steps executed)

	// Operator events
	TypeDashboardCommand = "dashboard_command"
)

// EventsFile is the name of the raw events log.
const EventsFile = ".events.jsonl"

// Log writes an event to the events log.
// The event is appended to ~/gt/.events.jsonl.
// Returns nil if logging fails (events are best-effort).
func Log(eventType, actor string, payload map[string]interface{}, visibility string) error {
	return LogEvent(Event{
		Type:       eventType,
		Actor:      actor,
		Payload:    payload,
		Visibility: visibility,
	})
}

// LogFeed is a convenience wrapper for feed-visible events.
func LogFeed(eventType, actor string, payload map[string]interface{}) error {
	return Log(eventType, actor, payload, VisibilityFeed)
}

// LogAudit is a convenience wrapper for audit-only events.
func LogAudit(eventType, actor string, payload map[string]interface{}) error {
	return Log(eventType, actor, payload, VisibilityAudit)
}

// PrepareEvent applies the canonical event defaults and infers common fields
// from the actor/payload so every sink sees the same wide event.
func PrepareEvent(event Event) Event {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if event.Source == "" {
		event.Source = "gt"
	}
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.Kind == "" {
		event.Kind = event.Type
	}
	if event.Type == "" {
		event.Type = event.Kind
	}
	if event.Visibility == "" {
		event.Visibility = VisibilityAudit
	}
	if event.Payload == nil {
		event.Payload = map[string]interface{}{}
	}
	if event.Evidence == nil {
		event.Evidence = map[string]interface{}{}
	}
	if event.Rig == "" {
		event.Rig = payloadString(event.Payload, "rig")
		if event.Rig == "" {
			event.Rig = inferRigFromActor(event.Actor)
		}
	}
	if event.Session == "" {
		event.Session = payloadString(event.Payload, "session")
	}
	if event.BeadID == "" {
		event.BeadID = firstPayloadString(event.Payload, "bead_id", "bead")
	}
	if event.MRID == "" {
		event.MRID = firstPayloadString(event.Payload, "mr_id", "mr")
	}
	if event.ConvoyID == "" {
		event.ConvoyID = firstPayloadString(event.Payload, "convoy_id", "convoy")
	}
	if event.Reason == "" {
		event.Reason = firstPayloadString(event.Payload, "reason", "error")
	}
	if event.DurationMs == 0 {
		event.DurationMs = payloadInt64(event.Payload, "duration_ms")
	}
	return event
}

// LogEvent writes a canonical event to the current town root.
func LogEvent(event Event) error {
	return writeAt("", PrepareEvent(event))
}

// LogEventAt writes a canonical event to a specific town root.
func LogEventAt(townRoot string, event Event) error {
	return writeAt(townRoot, PrepareEvent(event))
}

// write appends an event to the events file.
// Uses flock for cross-process synchronization — sync.Mutex only protects
// intra-process goroutines, but multiple gt processes write concurrently.
func write(event Event) error {
	return writeAt("", PrepareEvent(event))
}

func writeAt(townRoot string, event Event) error {
	if townRoot == "" {
		var err error
		townRoot, err = detectTownRoot()
		if err != nil || townRoot == "" {
			// Silently ignore - we're not in a Gas Town workspace
			return nil
		}
	}

	eventsPath := filepath.Join(townRoot, EventsFile)

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil || townRoot == "" {
		return fmt.Errorf("marshaling event: %w", err)
	}
	data = append(data, '\n')

	// Acquire cross-process file lock
	fl := flock.New(eventsPath + ".lock")
	if err := fl.Lock(); err != nil {
		return fmt.Errorf("acquiring events file lock: %w", err)
	}
	defer fl.Unlock() //nolint:errcheck // best-effort unlock

	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G302: events file is non-sensitive operational data
	if err != nil {
		return fmt.Errorf("opening events file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing event: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("closing events file: %w", err)
	}

	if store, err := controlplane.Open(townRoot); err == nil {
		_ = store.RecordEvent(toTownEvent(event))
	}

	return nil
}

func toTownEvent(event Event) controlplane.TownEvent {
	return controlplane.TownEvent{
		EventID:    event.EventID,
		Timestamp:  event.Timestamp,
		Kind:       event.Kind,
		Type:       event.Type,
		Actor:      event.Actor,
		Role:       event.Role,
		Rig:        event.Rig,
		Session:    event.Session,
		RunID:      event.RunID,
		BeadID:     event.BeadID,
		MRID:       event.MRID,
		ConvoyID:   event.ConvoyID,
		Outcome:    event.Outcome,
		Reason:     event.Reason,
		DurationMs: event.DurationMs,
		Payload:    event.Payload,
		Evidence:   event.Evidence,
		Visibility: event.Visibility,
		Source:     event.Source,
	}
}

func detectTownRoot() (string, error) {
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		return townRoot, nil
	}
	for _, envName := range []string{"GT_TOWN_ROOT", "GT_ROOT"} {
		if townRoot := os.Getenv(envName); townRoot != "" {
			if ok, _ := workspace.IsWorkspace(townRoot); ok {
				return townRoot, nil
			}
		}
	}
	return "", workspace.ErrNotFound
}

func firstPayloadString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := payloadString(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func payloadString(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func payloadInt64(payload map[string]interface{}, key string) int64 {
	if payload == nil {
		return 0
	}
	value, ok := payload[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	default:
		return 0
	}
}

func inferRigFromActor(actor string) string {
	if actor == "" {
		return ""
	}
	if actor == "mayor" || actor == "deacon" || actor == "daemon" || actor == "gt" || actor == "dashboard" {
		return ""
	}
	if slash := filepath.ToSlash(actor); slash != "" {
		parts := filepath.SplitList("")
		_ = parts
	}
	parts := splitActor(actor)
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "mayor", "deacon", "daemon", "gt", "dashboard":
		return ""
	default:
		return parts[0]
	}
}

func splitActor(actor string) []string {
	var parts []string
	current := ""
	for _, r := range actor {
		if r == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
			continue
		}
		current += string(r)
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// Payload helpers for common event structures.

// SlingPayload creates a payload for sling events.
func SlingPayload(beadID, target string) map[string]interface{} {
	return map[string]interface{}{
		"bead":   beadID,
		"target": target,
	}
}

// HookPayload creates a payload for hook events.
func HookPayload(beadID string) map[string]interface{} {
	return map[string]interface{}{
		"bead": beadID,
	}
}

// HandoffPayload creates a payload for handoff events.
func HandoffPayload(subject string, toSession bool) map[string]interface{} {
	p := map[string]interface{}{
		"to_session": toSession,
	}
	if subject != "" {
		p["subject"] = subject
	}
	return p
}

// DonePayload creates a payload for done events.
func DonePayload(beadID, branch string) map[string]interface{} {
	return map[string]interface{}{
		"bead":   beadID,
		"branch": branch,
	}
}

// MailPayload creates a payload for mail events.
func MailPayload(to, subject string) map[string]interface{} {
	return map[string]interface{}{
		"to":      to,
		"subject": subject,
	}
}

// SpawnPayload creates a payload for spawn events.
func SpawnPayload(rig, polecat string) map[string]interface{} {
	return map[string]interface{}{
		"rig":     rig,
		"polecat": polecat,
	}
}

// BootPayload creates a payload for rig boot events.
func BootPayload(rig string, agents []string) map[string]interface{} {
	return map[string]interface{}{
		"rig":    rig,
		"agents": agents,
	}
}

// MergePayload creates a payload for merge queue events.
// mrID: merge request ID
// worker: polecat name that submitted the work
// branch: source branch being merged
// reason: failure reason (for merge_failed/merge_skipped events)
func MergePayload(mrID, worker, branch, reason string) map[string]interface{} {
	p := map[string]interface{}{
		"mr":     mrID,
		"worker": worker,
		"branch": branch,
	}
	if reason != "" {
		p["reason"] = reason
	}
	return p
}

// PatrolPayload creates a payload for patrol start/complete events.
func PatrolPayload(rig string, polecatCount int, message string) map[string]interface{} {
	p := map[string]interface{}{
		"rig":           rig,
		"polecat_count": polecatCount,
	}
	if message != "" {
		p["message"] = message
	}
	return p
}

// PolecatCheckPayload creates a payload for polecat check events.
func PolecatCheckPayload(rig, polecat, status, issue string) map[string]interface{} {
	p := map[string]interface{}{
		"rig":     rig,
		"polecat": polecat,
		"status":  status,
	}
	if issue != "" {
		p["issue"] = issue
	}
	return p
}

// NudgePayload creates a payload for nudge events.
func NudgePayload(rig, target, reason string) map[string]interface{} {
	return map[string]interface{}{
		"rig":    rig,
		"target": target,
		"reason": reason,
	}
}

// EscalationPayload creates a payload for escalation events.
func EscalationPayload(rig, target, to, reason string) map[string]interface{} {
	return map[string]interface{}{
		"rig":    rig,
		"target": target,
		"to":     to,
		"reason": reason,
	}
}

// UnhookPayload creates a payload for unhook events.
func UnhookPayload(beadID string) map[string]interface{} {
	return map[string]interface{}{
		"bead": beadID,
	}
}

// KillPayload creates a payload for kill events.
func KillPayload(rig, target, reason string) map[string]interface{} {
	return map[string]interface{}{
		"rig":    rig,
		"target": target,
		"reason": reason,
	}
}

// HaltPayload creates a payload for halt events.
func HaltPayload(services []string) map[string]interface{} {
	return map[string]interface{}{
		"services": services,
	}
}

// SessionDeathPayload creates a payload for session death events.
// session: tmux session name that died
// agent: Gas Town agent identity (e.g., "gastown/polecats/Toast")
// reason: why the session was killed (e.g., "zombie cleanup", "user request", "doctor fix")
// caller: what initiated the kill (e.g., "daemon", "doctor", "gt down")
func SessionDeathPayload(session, agent, reason, caller string) map[string]interface{} {
	return map[string]interface{}{
		"session": session,
		"agent":   agent,
		"reason":  reason,
		"caller":  caller,
	}
}

// MassDeathPayload creates a payload for mass death events.
// count: number of sessions that died
// window: time window in which deaths occurred (e.g., "5s")
// sessions: list of session names that died
// possibleCause: suspected cause if known
func MassDeathPayload(count int, window string, sessions []string, possibleCause string) map[string]interface{} {
	p := map[string]interface{}{
		"count":    count,
		"window":   window,
		"sessions": sessions,
	}
	if possibleCause != "" {
		p["possible_cause"] = possibleCause
	}
	return p
}

// SessionPayload creates a payload for session start/end events.
// sessionID: Claude Code session UUID
// role: Gas Town role (e.g., "gastown/crew/joe", "deacon")
// topic: What the session is working on
// cwd: Working directory
func SessionPayload(sessionID, role, topic, cwd string) map[string]interface{} {
	p := map[string]interface{}{
		"session_id": sessionID,
		"role":       role,
		"actor_pid":  fmt.Sprintf("%s-%d", role, os.Getpid()),
	}
	if topic != "" {
		p["topic"] = topic
	}
	if cwd != "" {
		p["cwd"] = cwd
	}
	return p
}

// SchedulerEnqueuePayload creates a payload for scheduler enqueue events.
func SchedulerEnqueuePayload(beadID, rig string) map[string]interface{} {
	return map[string]interface{}{
		"bead": beadID,
		"rig":  rig,
	}
}

// SchedulerDispatchPayload creates a payload for scheduler dispatch events.
func SchedulerDispatchPayload(beadID, rig, polecat string) map[string]interface{} {
	return map[string]interface{}{
		"bead":    beadID,
		"rig":     rig,
		"polecat": polecat,
	}
}

// SchedulerDispatchFailedPayload creates a payload for scheduler dispatch failure events.
func SchedulerDispatchFailedPayload(beadID, rig, errMsg string) map[string]interface{} {
	return map[string]interface{}{
		"bead":  beadID,
		"rig":   rig,
		"error": errMsg,
	}
}
