package controlplane

import "errors"

var ErrLeaseHeld = errors.New("lease already held")
var ErrSQLiteUnavailable = errors.New("sqlite3 CLI not available")

// TownEvent is the canonical operator/audit event shape.
//
// It is intentionally wider than the legacy .events.jsonl schema so runtime
// behavior can be reconstructed without guessing from scattered state.
type TownEvent struct {
	EventID    string                 `json:"event_id,omitempty"`
	Timestamp  string                 `json:"ts"`
	Kind       string                 `json:"kind,omitempty"`
	Type       string                 `json:"type,omitempty"`
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
	Source     string                 `json:"source"`
}

// AgentRuntimeRecord is the control-plane projection of a session/agent.
type AgentRuntimeRecord struct {
	AgentID         string `json:"agent_id"`
	Role            string `json:"role,omitempty"`
	Rig             string `json:"rig,omitempty"`
	AgentName       string `json:"agent_name,omitempty"`
	Session         string `json:"session,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	WorkDir         string `json:"work_dir,omitempty"`
	Status          string `json:"status"`
	StatusReason    string `json:"status_reason,omitempty"`
	SourceAgreement string `json:"source_agreement,omitempty"`
	LastEventID     string `json:"last_event_id,omitempty"`
	LastEventKind   string `json:"last_event_kind,omitempty"`
	LastEventTS     string `json:"last_event_ts,omitempty"`
	UpdatedAt       string `json:"updated_at"`
}

// Incident is a user-facing issue derived from the canonical event stream.
type Incident struct {
	ID        string `json:"id"`
	EventID   string `json:"event_id,omitempty"`
	Timestamp string `json:"ts"`
	Kind      string `json:"kind"`
	Severity  string `json:"severity"`
	Actor     string `json:"actor"`
	Rig       string `json:"rig,omitempty"`
	Session   string `json:"session,omitempty"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
}

// LeaseRecord tracks singleton ownership for supervisor-like services.
type LeaseRecord struct {
	LeaseID    string                 `json:"lease_id"`
	Service    string                 `json:"service"`
	Rig        string                 `json:"rig,omitempty"`
	Session    string                 `json:"session,omitempty"`
	Holder     string                 `json:"holder,omitempty"`
	Status     string                 `json:"status"`
	AcquiredAt string                 `json:"acquired_at"`
	RenewedAt  string                 `json:"renewed_at"`
	ReleasedAt string                 `json:"released_at,omitempty"`
	Detail     string                 `json:"detail,omitempty"`
	Evidence   map[string]interface{} `json:"evidence,omitempty"`
}

// RespawnCounter is the authoritative supervisor counter for witness respawns.
type RespawnCounter struct {
	BeadID      string                 `json:"bead_id"`
	Rig         string                 `json:"rig,omitempty"`
	Count       int                    `json:"count"`
	MaxCount    int                    `json:"max_count,omitempty"`
	LastRespawn string                 `json:"last_respawn,omitempty"`
	Blocked     bool                   `json:"blocked"`
	UpdatedAt   string                 `json:"updated_at"`
	Evidence    map[string]interface{} `json:"evidence,omitempty"`
}

// RedispatchRecord is the authoritative state for deacon redispatch decisions.
type RedispatchRecord struct {
	BeadID          string                 `json:"bead_id"`
	SourceRig       string                 `json:"source_rig,omitempty"`
	TargetRig       string                 `json:"target_rig,omitempty"`
	AttemptCount    int                    `json:"attempt_count"`
	LastAttemptTime string                 `json:"last_attempt_time,omitempty"`
	CooldownUntil   string                 `json:"cooldown_until,omitempty"`
	Escalated       bool                   `json:"escalated"`
	EscalatedAt     string                 `json:"escalated_at,omitempty"`
	LastAction      string                 `json:"last_action,omitempty"`
	UpdatedAt       string                 `json:"updated_at"`
	Evidence        map[string]interface{} `json:"evidence,omitempty"`
}

// CleanupState tracks durable cleanup progress and blockers for a polecat.
type CleanupState struct {
	CleanupID    string                 `json:"cleanup_id"`
	Rig          string                 `json:"rig,omitempty"`
	PolecatName  string                 `json:"polecat_name,omitempty"`
	BeadID       string                 `json:"bead_id,omitempty"`
	Session      string                 `json:"session,omitempty"`
	Status       string                 `json:"status"`
	Blocker      string                 `json:"blocker,omitempty"`
	WispID       string                 `json:"wisp_id,omitempty"`
	AttemptCount int                    `json:"attempt_count,omitempty"`
	LastError    string                 `json:"last_error,omitempty"`
	UpdatedAt    string                 `json:"updated_at"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
}

// DependencyHealth is the latest control-plane view of an external dependency.
type DependencyHealth struct {
	DependencyKey string                 `json:"dependency_key"`
	Name          string                 `json:"name"`
	Scope         string                 `json:"scope,omitempty"`
	Status        string                 `json:"status"`
	Detail        string                 `json:"detail,omitempty"`
	CheckedAt     string                 `json:"checked_at"`
	LastHealthyAt string                 `json:"last_healthy_at,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
}

// LeaseKey returns the stable primary key for a singleton lease.
func LeaseKey(service, rig string) string {
	if rig == "" {
		return service
	}
	return service + ":" + rig
}

// CleanupKey returns the stable primary key for cleanup state tied to a polecat.
func CleanupKey(rig, polecatName string) string {
	if polecatName == "" {
		return rig
	}
	return rig + "/" + polecatName
}

// DependencyKey returns the stable primary key for a dependency in a scope.
func DependencyKey(name, scope string) string {
	if scope == "" {
		return name
	}
	return name + ":" + scope
}
