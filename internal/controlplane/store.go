package controlplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	controlPlaneDir = ".controlplane"
	dbName          = "controlplane.db"
)

const schemaSQL = `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS events (
  event_id TEXT PRIMARY KEY,
  ts TEXT NOT NULL,
  kind TEXT NOT NULL,
  event_type TEXT NOT NULL,
  actor TEXT NOT NULL,
  role TEXT,
  rig TEXT,
  session TEXT,
  run_id TEXT,
  bead_id TEXT,
  mr_id TEXT,
  convoy_id TEXT,
  outcome TEXT,
  reason TEXT,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  visibility TEXT NOT NULL,
  source TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  evidence_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session, ts DESC);
CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind, ts DESC);
CREATE TABLE IF NOT EXISTS agents (
  agent_id TEXT PRIMARY KEY,
  role TEXT,
  rig TEXT,
  agent_name TEXT,
  session TEXT,
  run_id TEXT,
  work_dir TEXT,
  status TEXT NOT NULL,
  status_reason TEXT,
  source_agreement TEXT,
  last_event_id TEXT,
  last_event_kind TEXT,
  last_event_ts TEXT,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agents_session ON agents(session);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE TABLE IF NOT EXISTS leases (
  lease_id TEXT PRIMARY KEY,
  service TEXT NOT NULL,
  rig TEXT,
  session TEXT,
  holder TEXT,
  status TEXT NOT NULL,
  acquired_at TEXT NOT NULL,
  renewed_at TEXT NOT NULL,
  released_at TEXT,
  detail TEXT,
  evidence_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_leases_service ON leases(service, rig);
CREATE TABLE IF NOT EXISTS respawn_counters (
  bead_id TEXT PRIMARY KEY,
  rig TEXT,
  count INTEGER NOT NULL DEFAULT 0,
  max_count INTEGER NOT NULL DEFAULT 0,
  last_respawn TEXT,
  blocked INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_respawn_rig ON respawn_counters(rig, updated_at DESC);
CREATE TABLE IF NOT EXISTS redispatch_state (
  bead_id TEXT PRIMARY KEY,
  source_rig TEXT,
  target_rig TEXT,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_attempt_time TEXT,
  cooldown_until TEXT,
  escalated INTEGER NOT NULL DEFAULT 0,
  escalated_at TEXT,
  last_action TEXT,
  updated_at TEXT NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_redispatch_updated ON redispatch_state(updated_at DESC);
CREATE TABLE IF NOT EXISTS cleanup_state (
  cleanup_id TEXT PRIMARY KEY,
  rig TEXT,
  polecat_name TEXT,
  bead_id TEXT,
  session TEXT,
  status TEXT NOT NULL,
  blocker TEXT,
  wisp_id TEXT,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  updated_at TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_cleanup_rig ON cleanup_state(rig, updated_at DESC);
CREATE TABLE IF NOT EXISTS dependency_health (
  dependency_key TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  scope TEXT,
  status TEXT NOT NULL,
  detail TEXT,
  checked_at TEXT NOT NULL,
  last_healthy_at TEXT,
  payload_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_dependency_status ON dependency_health(status, checked_at DESC);
`

// Store is a lightweight adapter over a local SQLite control-plane database.
//
// The current implementation intentionally uses the system sqlite3 CLI so the
// repo can gain a real SQLite-backed authority without pulling a new Go driver
// into the module in the same change.
type Store struct {
	townRoot string
	dbPath   string
}

// DBPath returns the canonical control-plane SQLite path.
func DBPath(townRoot string) string {
	return filepath.Join(townRoot, controlPlaneDir, dbName)
}

// Open ensures the control-plane database exists and is ready.
func Open(townRoot string) (*Store, error) {
	if townRoot == "" {
		return nil, fmt.Errorf("town root is required")
	}
	dir := filepath.Dir(DBPath(townRoot))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating control-plane dir: %w", err)
	}
	s := &Store{
		townRoot: townRoot,
		dbPath:   DBPath(townRoot),
	}
	if err := s.exec(schemaSQL); err != nil {
		return nil, err
	}
	return s, nil
}

// RecordEvent appends or updates an event in the control-plane store.
func (s *Store) RecordEvent(event TownEvent) error {
	payloadJSON, err := marshalJSONText(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	evidenceJSON, err := marshalJSONText(event.Evidence)
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}

	sql := fmt.Sprintf(`
INSERT INTO events (
  event_id, ts, kind, event_type, actor, role, rig, session, run_id,
  bead_id, mr_id, convoy_id, outcome, reason, duration_ms, visibility,
  source, payload_json, evidence_json
) VALUES (
  %s, %s, %s, %s, %s, %s, %s, %s, %s,
  %s, %s, %s, %s, %s, %d, %s,
  %s, %s, %s
)
ON CONFLICT(event_id) DO UPDATE SET
  ts=excluded.ts,
  kind=excluded.kind,
  event_type=excluded.event_type,
  actor=excluded.actor,
  role=excluded.role,
  rig=excluded.rig,
  session=excluded.session,
  run_id=excluded.run_id,
  bead_id=excluded.bead_id,
  mr_id=excluded.mr_id,
  convoy_id=excluded.convoy_id,
  outcome=excluded.outcome,
  reason=excluded.reason,
  duration_ms=excluded.duration_ms,
  visibility=excluded.visibility,
  source=excluded.source,
  payload_json=excluded.payload_json,
  evidence_json=excluded.evidence_json;
`, sqlString(event.EventID), sqlString(event.Timestamp), sqlString(event.Kind),
		sqlString(event.Type), sqlString(event.Actor), sqlString(event.Role),
		sqlString(event.Rig), sqlString(event.Session), sqlString(event.RunID),
		sqlString(event.BeadID), sqlString(event.MRID), sqlString(event.ConvoyID),
		sqlString(event.Outcome), sqlString(event.Reason), event.DurationMs,
		sqlString(event.Visibility), sqlString(event.Source), sqlString(payloadJSON),
		sqlString(evidenceJSON))

	return s.exec(sql)
}

// UpsertAgentRuntime writes the current runtime projection for an agent/session.
func (s *Store) UpsertAgentRuntime(record AgentRuntimeRecord) error {
	sql := fmt.Sprintf(`
INSERT INTO agents (
  agent_id, role, rig, agent_name, session, run_id, work_dir, status,
  status_reason, source_agreement, last_event_id, last_event_kind,
  last_event_ts, updated_at
) VALUES (
  %s, %s, %s, %s, %s, %s, %s, %s,
  %s, %s, %s, %s,
  %s, %s
)
ON CONFLICT(agent_id) DO UPDATE SET
  role=excluded.role,
  rig=excluded.rig,
  agent_name=excluded.agent_name,
  session=excluded.session,
  run_id=excluded.run_id,
  work_dir=excluded.work_dir,
  status=excluded.status,
  status_reason=excluded.status_reason,
  source_agreement=excluded.source_agreement,
  last_event_id=excluded.last_event_id,
  last_event_kind=excluded.last_event_kind,
  last_event_ts=excluded.last_event_ts,
  updated_at=excluded.updated_at;
`, sqlString(record.AgentID), sqlString(record.Role), sqlString(record.Rig),
		sqlString(record.AgentName), sqlString(record.Session), sqlString(record.RunID),
		sqlString(record.WorkDir), sqlString(record.Status), sqlString(record.StatusReason),
		sqlString(record.SourceAgreement), sqlString(record.LastEventID),
		sqlString(record.LastEventKind), sqlString(record.LastEventTS),
		sqlString(record.UpdatedAt))
	return s.exec(sql)
}

// ListEvents returns recent events ordered newest-first.
func (s *Store) ListEvents(limit int) ([]TownEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []eventRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT event_id, ts, kind, event_type, actor, role, rig, session, run_id,
       bead_id, mr_id, convoy_id, outcome, reason, duration_ms, visibility,
       source, payload_json, evidence_json
FROM events
ORDER BY ts DESC
LIMIT %d;
`, limit), &rows); err != nil {
		return nil, err
	}
	return decodeEventRows(rows)
}

// ListEventsBySession returns recent events for a session.
func (s *Store) ListEventsBySession(sessionID string, limit int) ([]TownEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []eventRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT event_id, ts, kind, event_type, actor, role, rig, session, run_id,
       bead_id, mr_id, convoy_id, outcome, reason, duration_ms, visibility,
       source, payload_json, evidence_json
FROM events
WHERE session = %s
ORDER BY ts DESC
LIMIT %d;
`, sqlString(sessionID), limit), &rows); err != nil {
		return nil, err
	}
	return decodeEventRows(rows)
}

// ListAgentRuntime returns all known runtime records newest-first.
func (s *Store) ListAgentRuntime() ([]AgentRuntimeRecord, error) {
	var rows []AgentRuntimeRecord
	if err := s.queryJSON(`
SELECT agent_id, role, rig, agent_name, session, run_id, work_dir, status,
       status_reason, source_agreement, last_event_id, last_event_kind,
       last_event_ts, updated_at
FROM agents
ORDER BY updated_at DESC;
`, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetAgentRuntime looks up a specific agent/session record.
func (s *Store) GetAgentRuntime(agentID string) (*AgentRuntimeRecord, error) {
	var rows []AgentRuntimeRecord
	if err := s.queryJSON(fmt.Sprintf(`
SELECT agent_id, role, rig, agent_name, session, run_id, work_dir, status,
       status_reason, source_agreement, last_event_id, last_event_kind,
       last_event_ts, updated_at
FROM agents
WHERE agent_id = %s OR session = %s
LIMIT 1;
`, sqlString(agentID), sqlString(agentID)), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// ListIncidents derives current incident candidates from the canonical event log.
func (s *Store) ListIncidents(limit int) ([]Incident, error) {
	if limit <= 0 {
		limit = 50
	}
	events, err := s.ListEvents(limit * 4)
	if err != nil {
		return nil, err
	}
	incidents := make([]Incident, 0, limit)
	for _, event := range events {
		if !isIncidentEvent(event) {
			continue
		}
		incidents = append(incidents, Incident{
			ID:        incidentID(event),
			EventID:   event.EventID,
			Timestamp: event.Timestamp,
			Kind:      event.Kind,
			Severity:  severityForEvent(event),
			Actor:     event.Actor,
			Rig:       event.Rig,
			Session:   event.Session,
			Summary:   summaryForEvent(event),
			Status:    incidentStatusForEvent(event),
			Reason:    event.Reason,
		})
		if len(incidents) >= limit {
			break
		}
	}
	return incidents, nil
}

type eventRow struct {
	EventID      string `json:"event_id"`
	Timestamp    string `json:"ts"`
	Kind         string `json:"kind"`
	Type         string `json:"event_type"`
	Actor        string `json:"actor"`
	Role         string `json:"role"`
	Rig          string `json:"rig"`
	Session      string `json:"session"`
	RunID        string `json:"run_id"`
	BeadID       string `json:"bead_id"`
	MRID         string `json:"mr_id"`
	ConvoyID     string `json:"convoy_id"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason"`
	DurationMs   int64  `json:"duration_ms"`
	Visibility   string `json:"visibility"`
	Source       string `json:"source"`
	PayloadJSON  string `json:"payload_json"`
	EvidenceJSON string `json:"evidence_json"`
}

func decodeEventRows(rows []eventRow) ([]TownEvent, error) {
	events := make([]TownEvent, 0, len(rows))
	for _, row := range rows {
		payload, err := unmarshalJSONMap(row.PayloadJSON)
		if err != nil {
			return nil, err
		}
		evidence, err := unmarshalJSONMap(row.EvidenceJSON)
		if err != nil {
			return nil, err
		}
		events = append(events, TownEvent{
			EventID:    row.EventID,
			Timestamp:  row.Timestamp,
			Kind:       row.Kind,
			Type:       row.Type,
			Actor:      row.Actor,
			Role:       row.Role,
			Rig:        row.Rig,
			Session:    row.Session,
			RunID:      row.RunID,
			BeadID:     row.BeadID,
			MRID:       row.MRID,
			ConvoyID:   row.ConvoyID,
			Outcome:    row.Outcome,
			Reason:     row.Reason,
			DurationMs: row.DurationMs,
			Payload:    payload,
			Evidence:   evidence,
			Visibility: row.Visibility,
			Source:     row.Source,
		})
	}
	return events, nil
}

func (s *Store) exec(sql string) error {
	cmd := exec.Command("sqlite3", s.dbPath, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("sqlite3 exec: %v: %s", err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("sqlite3 exec: %w", err)
	}
	return nil
}

func (s *Store) queryJSON(sql string, out interface{}) error {
	cmd := exec.Command("sqlite3", "-json", s.dbPath, sql)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("sqlite3 query: %v: %s", err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("sqlite3 query: %w", err)
	}
	if stdout.Len() == 0 {
		return nil
	}
	return json.Unmarshal(stdout.Bytes(), out)
}

func marshalJSONText(value map[string]interface{}) (string, error) {
	if len(value) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalJSONMap(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("decode json map: %w", err)
	}
	return out, nil
}

func sqlString(value string) string {
	if value == "" {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func isIncidentEvent(event TownEvent) bool {
	switch event.Kind {
	case "session_death", "mass_death", "merge_failed", "scheduler_dispatch_failed", "escalation_sent":
		return true
	}
	return event.Outcome == "error"
}

func incidentID(event TownEvent) string {
	if event.EventID != "" {
		return "incident-" + event.EventID
	}
	return "incident-" + strconv.FormatInt(event.DurationMs, 10)
}

func severityForEvent(event TownEvent) string {
	switch event.Kind {
	case "mass_death":
		return "critical"
	case "session_death", "merge_failed", "scheduler_dispatch_failed":
		return "high"
	case "escalation_sent":
		return "medium"
	default:
		if event.Outcome == "error" {
			return "high"
		}
		return "medium"
	}
}

func summaryForEvent(event TownEvent) string {
	if event.Reason != "" {
		return fmt.Sprintf("%s: %s", event.Kind, event.Reason)
	}
	if event.Session != "" {
		return fmt.Sprintf("%s for %s", event.Kind, event.Session)
	}
	return event.Kind
}

func incidentStatusForEvent(event TownEvent) string {
	if event.Kind == "escalation_closed" {
		return "resolved"
	}
	if event.Outcome == "success" {
		return "resolved"
	}
	return "open"
}
