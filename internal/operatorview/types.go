package operatorview

import "github.com/steveyegge/gastown/internal/controlplane"

// Incident is the operator-facing incident type derived from canonical events.
type Incident = controlplane.Incident

// LeaseRecord is the operator-facing singleton lease type.
type LeaseRecord = controlplane.LeaseRecord

// RespawnCounter is the operator-facing witness respawn counter.
type RespawnCounter = controlplane.RespawnCounter

// RedispatchRecord is the operator-facing deacon redispatch state.
type RedispatchRecord = controlplane.RedispatchRecord

// CleanupState is the operator-facing cleanup state projection.
type CleanupState = controlplane.CleanupState

// DependencyHealth is the operator-facing dependency health projection.
type DependencyHealth = controlplane.DependencyHealth

// ProjectionStatus reports the health of a single evidence source or projection.
type ProjectionStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// SupervisorDecision captures a visible supervisor action derived from events.
type SupervisorDecision struct {
	Timestamp string `json:"ts"`
	Kind      string `json:"kind"`
	Actor     string `json:"actor"`
	Role      string `json:"role,omitempty"`
	Rig       string `json:"rig,omitempty"`
	Session   string `json:"session,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// TmuxProjection reports what tmux says about a session.
type TmuxProjection struct {
	Exists   bool   `json:"exists"`
	Session  string `json:"session,omitempty"`
	WorkDir  string `json:"work_dir,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	Role     string `json:"role,omitempty"`
	Rig      string `json:"rig,omitempty"`
	Agent    string `json:"agent,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`
}

// HeartbeatProjection reports the agent heartbeat view.
type HeartbeatProjection struct {
	Exists    bool   `json:"exists"`
	Fresh     bool   `json:"fresh"`
	Timestamp string `json:"ts,omitempty"`
	State     string `json:"state,omitempty"`
	Context   string `json:"context,omitempty"`
	Bead      string `json:"bead,omitempty"`
}

// CheckpointProjection reports checkpoint recovery state.
type CheckpointProjection struct {
	Exists      bool   `json:"exists"`
	Path        string `json:"path,omitempty"`
	Timestamp   string `json:"ts,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Summary     string `json:"summary,omitempty"`
	HookedBead  string `json:"hooked_bead,omitempty"`
	CurrentStep string `json:"current_step,omitempty"`
}

// BeadsProjection reports the agent bead state if present.
type BeadsProjection struct {
	Exists          bool   `json:"exists"`
	AgentBeadID     string `json:"agent_bead_id,omitempty"`
	AgentState      string `json:"agent_state,omitempty"`
	HookBead        string `json:"hook_bead,omitempty"`
	ActiveMR        string `json:"active_mr,omitempty"`
	CleanupStatus   string `json:"cleanup_status,omitempty"`
	CompletionTime  string `json:"completion_time,omitempty"`
	NotificationLvl string `json:"notification_level,omitempty"`
}

// PIDProjection reports whether a tracked PID file exists for the session.
type PIDProjection struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path,omitempty"`
}

// AgentSnapshot is the reconciled operator view for one agent/session.
type AgentSnapshot struct {
	AgentID         string                           `json:"agent_id"`
	Role            string                           `json:"role,omitempty"`
	Rig             string                           `json:"rig,omitempty"`
	AgentName       string                           `json:"agent_name,omitempty"`
	Session         string                           `json:"session,omitempty"`
	RunID           string                           `json:"run_id,omitempty"`
	WorkDir         string                           `json:"work_dir,omitempty"`
	Status          string                           `json:"status"`
	StatusReason    string                           `json:"status_reason,omitempty"`
	SourceAgreement string                           `json:"source_agreement,omitempty"`
	Conflicts       []string                         `json:"conflicts,omitempty"`
	Projections     []ProjectionStatus               `json:"projections,omitempty"`
	Runtime         *controlplane.AgentRuntimeRecord `json:"runtime,omitempty"`
	Lease           *LeaseRecord                     `json:"lease,omitempty"`
	Respawn         *RespawnCounter                  `json:"respawn,omitempty"`
	Redispatch      *RedispatchRecord                `json:"redispatch,omitempty"`
	Cleanup         *CleanupState                    `json:"cleanup,omitempty"`
	Tmux            TmuxProjection                   `json:"tmux"`
	Heartbeat       HeartbeatProjection              `json:"heartbeat"`
	Checkpoint      CheckpointProjection             `json:"checkpoint"`
	Beads           BeadsProjection                  `json:"beads"`
	PID             PIDProjection                    `json:"pid"`
	RecentEvents    []controlplane.TownEvent         `json:"recent_events,omitempty"`
	Decisions       []SupervisorDecision             `json:"decisions,omitempty"`
}

// TownSnapshot is the top-level reconciled operator view for a town.
type TownSnapshot struct {
	GeneratedAt     string                   `json:"generated_at"`
	TownRoot        string                   `json:"town_root"`
	Status          string                   `json:"status"`
	StatusReason    string                   `json:"status_reason,omitempty"`
	SourceAgreement string                   `json:"source_agreement,omitempty"`
	Conflicts       []string                 `json:"conflicts,omitempty"`
	Projections     []ProjectionStatus       `json:"projections,omitempty"`
	Leases          []LeaseRecord            `json:"leases,omitempty"`
	Respawns        []RespawnCounter         `json:"respawns,omitempty"`
	Redispatches    []RedispatchRecord       `json:"redispatches,omitempty"`
	CleanupStates   []CleanupState           `json:"cleanup_states,omitempty"`
	Dependencies    []DependencyHealth       `json:"dependencies,omitempty"`
	Agents          []AgentSnapshot          `json:"agents"`
	RecentEvents    []controlplane.TownEvent `json:"recent_events,omitempty"`
	Incidents       []Incident               `json:"incidents,omitempty"`
}
