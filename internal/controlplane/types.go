package controlplane

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
