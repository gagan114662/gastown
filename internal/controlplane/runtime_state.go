package controlplane

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AcquireLease records active ownership for a singleton service.
func (s *Store) AcquireLease(record LeaseRecord) (*LeaseRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if record.LeaseID == "" {
		record.LeaseID = LeaseKey(record.Service, record.Rig)
	}
	if record.Status == "" {
		record.Status = "active"
	}
	if record.AcquiredAt == "" {
		record.AcquiredAt = now
	}
	if record.RenewedAt == "" {
		record.RenewedAt = now
	}
	current, err := s.GetLease(record.LeaseID)
	if err != nil {
		return nil, err
	}
	if current != nil && current.Status == "active" && current.ReleasedAt == "" && current.Session != "" && current.Session != record.Session {
		return current, fmt.Errorf("%w: %s", ErrLeaseHeld, current.Session)
	}
	if err := s.UpsertLease(record); err != nil {
		return nil, err
	}
	return s.GetLease(record.LeaseID)
}

// UpsertLease writes the latest lease record.
func (s *Store) UpsertLease(record LeaseRecord) error {
	evidenceJSON, err := marshalJSONText(record.Evidence)
	if err != nil {
		return fmt.Errorf("marshal lease evidence: %w", err)
	}
	sql := fmt.Sprintf(`
INSERT INTO leases (
  lease_id, service, rig, session, holder, status, acquired_at, renewed_at,
  released_at, detail, evidence_json
) VALUES (
  %s, %s, %s, %s, %s, %s, %s, %s,
  %s, %s, %s
)
ON CONFLICT(lease_id) DO UPDATE SET
  service=excluded.service,
  rig=excluded.rig,
  session=excluded.session,
  holder=excluded.holder,
  status=excluded.status,
  acquired_at=excluded.acquired_at,
  renewed_at=excluded.renewed_at,
  released_at=excluded.released_at,
  detail=excluded.detail,
  evidence_json=excluded.evidence_json;
`, sqlString(record.LeaseID), sqlString(record.Service), sqlString(record.Rig),
		sqlString(record.Session), sqlString(record.Holder), sqlString(record.Status),
		sqlString(record.AcquiredAt), sqlString(record.RenewedAt), sqlString(record.ReleasedAt),
		sqlString(record.Detail), sqlString(evidenceJSON))
	return s.exec(sql)
}

// ReleaseLease marks a lease as released.
func (s *Store) ReleaseLease(leaseID, detail string) error {
	current, err := s.GetLease(leaseID)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	current.Status = "released"
	current.RenewedAt = now
	current.ReleasedAt = now
	if detail != "" {
		current.Detail = detail
	}
	return s.UpsertLease(*current)
}

// GetLease returns a single lease record.
func (s *Store) GetLease(leaseID string) (*LeaseRecord, error) {
	var rows []leaseRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT lease_id, service, rig, session, holder, status, acquired_at, renewed_at,
       released_at, detail, evidence_json
FROM leases
WHERE lease_id = %s
LIMIT 1;
`, sqlString(leaseID)), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	record, err := decodeLeaseRows(rows)
	if err != nil {
		return nil, err
	}
	return &record[0], nil
}

// ListLeases returns all leases newest-first.
func (s *Store) ListLeases() ([]LeaseRecord, error) {
	var rows []leaseRow
	if err := s.queryJSON(`
SELECT lease_id, service, rig, session, holder, status, acquired_at, renewed_at,
       released_at, detail, evidence_json
FROM leases
ORDER BY renewed_at DESC;
`, &rows); err != nil {
		return nil, err
	}
	return decodeLeaseRows(rows)
}

// UpsertRespawnCounter records the authoritative witness respawn counter.
func (s *Store) UpsertRespawnCounter(record RespawnCounter) error {
	evidenceJSON, err := marshalJSONText(record.Evidence)
	if err != nil {
		return fmt.Errorf("marshal respawn evidence: %w", err)
	}
	sql := fmt.Sprintf(`
INSERT INTO respawn_counters (
  bead_id, rig, count, max_count, last_respawn, blocked, updated_at, evidence_json
) VALUES (
  %s, %s, %d, %d, %s, %d, %s, %s
)
ON CONFLICT(bead_id) DO UPDATE SET
  rig=excluded.rig,
  count=excluded.count,
  max_count=excluded.max_count,
  last_respawn=excluded.last_respawn,
  blocked=excluded.blocked,
  updated_at=excluded.updated_at,
  evidence_json=excluded.evidence_json;
`, sqlString(record.BeadID), sqlString(record.Rig), record.Count, record.MaxCount,
		sqlString(record.LastRespawn), boolInt(record.Blocked), sqlString(record.UpdatedAt),
		sqlString(evidenceJSON))
	return s.exec(sql)
}

// GetRespawnCounter returns one witness respawn counter.
func (s *Store) GetRespawnCounter(beadID string) (*RespawnCounter, error) {
	var rows []respawnRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT bead_id, rig, count, max_count, last_respawn, blocked, updated_at, evidence_json
FROM respawn_counters
WHERE bead_id = %s
LIMIT 1;
`, sqlString(beadID)), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	records, err := decodeRespawnRows(rows)
	if err != nil {
		return nil, err
	}
	return &records[0], nil
}

// ListRespawnCounters returns recent respawn counters.
func (s *Store) ListRespawnCounters(limit int) ([]RespawnCounter, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []respawnRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT bead_id, rig, count, max_count, last_respawn, blocked, updated_at, evidence_json
FROM respawn_counters
ORDER BY updated_at DESC
LIMIT %d;
`, limit), &rows); err != nil {
		return nil, err
	}
	return decodeRespawnRows(rows)
}

// DeleteRespawnCounter removes the authoritative respawn counter.
func (s *Store) DeleteRespawnCounter(beadID string) error {
	return s.exec(fmt.Sprintf("DELETE FROM respawn_counters WHERE bead_id = %s;", sqlString(beadID)))
}

// UpsertRedispatchRecord records the authoritative deacon redispatch state.
func (s *Store) UpsertRedispatchRecord(record RedispatchRecord) error {
	evidenceJSON, err := marshalJSONText(record.Evidence)
	if err != nil {
		return fmt.Errorf("marshal redispatch evidence: %w", err)
	}
	sql := fmt.Sprintf(`
INSERT INTO redispatch_state (
  bead_id, source_rig, target_rig, attempt_count, last_attempt_time,
  cooldown_until, escalated, escalated_at, last_action, updated_at, evidence_json
) VALUES (
  %s, %s, %s, %d, %s,
  %s, %d, %s, %s, %s, %s
)
ON CONFLICT(bead_id) DO UPDATE SET
  source_rig=excluded.source_rig,
  target_rig=excluded.target_rig,
  attempt_count=excluded.attempt_count,
  last_attempt_time=excluded.last_attempt_time,
  cooldown_until=excluded.cooldown_until,
  escalated=excluded.escalated,
  escalated_at=excluded.escalated_at,
  last_action=excluded.last_action,
  updated_at=excluded.updated_at,
  evidence_json=excluded.evidence_json;
`, sqlString(record.BeadID), sqlString(record.SourceRig), sqlString(record.TargetRig),
		record.AttemptCount, sqlString(record.LastAttemptTime), sqlString(record.CooldownUntil),
		boolInt(record.Escalated), sqlString(record.EscalatedAt), sqlString(record.LastAction),
		sqlString(record.UpdatedAt), sqlString(evidenceJSON))
	return s.exec(sql)
}

// GetRedispatchRecord returns one authoritative redispatch record.
func (s *Store) GetRedispatchRecord(beadID string) (*RedispatchRecord, error) {
	var rows []redispatchRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT bead_id, source_rig, target_rig, attempt_count, last_attempt_time,
       cooldown_until, escalated, escalated_at, last_action, updated_at, evidence_json
FROM redispatch_state
WHERE bead_id = %s
LIMIT 1;
`, sqlString(beadID)), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	records, err := decodeRedispatchRows(rows)
	if err != nil {
		return nil, err
	}
	return &records[0], nil
}

// ListRedispatchRecords returns recent redispatch records.
func (s *Store) ListRedispatchRecords(limit int) ([]RedispatchRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []redispatchRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT bead_id, source_rig, target_rig, attempt_count, last_attempt_time,
       cooldown_until, escalated, escalated_at, last_action, updated_at, evidence_json
FROM redispatch_state
ORDER BY updated_at DESC
LIMIT %d;
`, limit), &rows); err != nil {
		return nil, err
	}
	return decodeRedispatchRows(rows)
}

// DeleteRedispatchRecord removes a redispatch record.
func (s *Store) DeleteRedispatchRecord(beadID string) error {
	return s.exec(fmt.Sprintf("DELETE FROM redispatch_state WHERE bead_id = %s;", sqlString(beadID)))
}

// UpsertCleanupState writes the latest cleanup state.
func (s *Store) UpsertCleanupState(state CleanupState) error {
	payloadJSON, err := marshalJSONText(state.Payload)
	if err != nil {
		return fmt.Errorf("marshal cleanup payload: %w", err)
	}
	sql := fmt.Sprintf(`
INSERT INTO cleanup_state (
  cleanup_id, rig, polecat_name, bead_id, session, status, blocker, wisp_id,
  attempt_count, last_error, updated_at, payload_json
) VALUES (
  %s, %s, %s, %s, %s, %s, %s, %s,
  %d, %s, %s, %s
)
ON CONFLICT(cleanup_id) DO UPDATE SET
  rig=excluded.rig,
  polecat_name=excluded.polecat_name,
  bead_id=excluded.bead_id,
  session=excluded.session,
  status=excluded.status,
  blocker=excluded.blocker,
  wisp_id=excluded.wisp_id,
  attempt_count=excluded.attempt_count,
  last_error=excluded.last_error,
  updated_at=excluded.updated_at,
  payload_json=excluded.payload_json;
`, sqlString(state.CleanupID), sqlString(state.Rig), sqlString(state.PolecatName),
		sqlString(state.BeadID), sqlString(state.Session), sqlString(state.Status),
		sqlString(state.Blocker), sqlString(state.WispID), state.AttemptCount,
		sqlString(state.LastError), sqlString(state.UpdatedAt), sqlString(payloadJSON))
	return s.exec(sql)
}

// GetCleanupState returns one cleanup state by ID.
func (s *Store) GetCleanupState(cleanupID string) (*CleanupState, error) {
	var rows []cleanupRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT cleanup_id, rig, polecat_name, bead_id, session, status, blocker, wisp_id,
       attempt_count, last_error, updated_at, payload_json
FROM cleanup_state
WHERE cleanup_id = %s
LIMIT 1;
`, sqlString(cleanupID)), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	states, err := decodeCleanupRows(rows)
	if err != nil {
		return nil, err
	}
	return &states[0], nil
}

// GetCleanupStateByPolecat returns cleanup state for a rig/polecat pair.
func (s *Store) GetCleanupStateByPolecat(rig, polecatName string) (*CleanupState, error) {
	return s.GetCleanupState(CleanupKey(rig, polecatName))
}

// ListCleanupStates returns recent cleanup states.
func (s *Store) ListCleanupStates(limit int) ([]CleanupState, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []cleanupRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT cleanup_id, rig, polecat_name, bead_id, session, status, blocker, wisp_id,
       attempt_count, last_error, updated_at, payload_json
FROM cleanup_state
ORDER BY updated_at DESC
LIMIT %d;
`, limit), &rows); err != nil {
		return nil, err
	}
	return decodeCleanupRows(rows)
}

// RecordDependencyHealth stores the latest health status for one dependency.
func (s *Store) RecordDependencyHealth(dep DependencyHealth) error {
	payloadJSON, err := marshalJSONText(dep.Payload)
	if err != nil {
		return fmt.Errorf("marshal dependency payload: %w", err)
	}
	if dep.DependencyKey == "" {
		dep.DependencyKey = DependencyKey(dep.Name, dep.Scope)
	}
	sql := fmt.Sprintf(`
INSERT INTO dependency_health (
  dependency_key, name, scope, status, detail, checked_at, last_healthy_at, payload_json
) VALUES (
  %s, %s, %s, %s, %s, %s, %s, %s
)
ON CONFLICT(dependency_key) DO UPDATE SET
  name=excluded.name,
  scope=excluded.scope,
  status=excluded.status,
  detail=excluded.detail,
  checked_at=excluded.checked_at,
  last_healthy_at=excluded.last_healthy_at,
  payload_json=excluded.payload_json;
`, sqlString(dep.DependencyKey), sqlString(dep.Name), sqlString(dep.Scope),
		sqlString(dep.Status), sqlString(dep.Detail), sqlString(dep.CheckedAt),
		sqlString(dep.LastHealthyAt), sqlString(payloadJSON))
	return s.exec(sql)
}

// GetDependencyHealth returns one dependency health record.
func (s *Store) GetDependencyHealth(key string) (*DependencyHealth, error) {
	var rows []dependencyRow
	if err := s.queryJSON(fmt.Sprintf(`
SELECT dependency_key, name, scope, status, detail, checked_at, last_healthy_at, payload_json
FROM dependency_health
WHERE dependency_key = %s
LIMIT 1;
`, sqlString(key)), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	records, err := decodeDependencyRows(rows)
	if err != nil {
		return nil, err
	}
	return &records[0], nil
}

// ListDependencyHealth returns all dependency health records newest-first.
func (s *Store) ListDependencyHealth() ([]DependencyHealth, error) {
	var rows []dependencyRow
	if err := s.queryJSON(`
SELECT dependency_key, name, scope, status, detail, checked_at, last_healthy_at, payload_json
FROM dependency_health
ORDER BY checked_at DESC;
`, &rows); err != nil {
		return nil, err
	}
	return decodeDependencyRows(rows)
}

type leaseRow struct {
	LeaseID      string `json:"lease_id"`
	Service      string `json:"service"`
	Rig          string `json:"rig"`
	Session      string `json:"session"`
	Holder       string `json:"holder"`
	Status       string `json:"status"`
	AcquiredAt   string `json:"acquired_at"`
	RenewedAt    string `json:"renewed_at"`
	ReleasedAt   string `json:"released_at"`
	Detail       string `json:"detail"`
	EvidenceJSON string `json:"evidence_json"`
}

type respawnRow struct {
	BeadID       string `json:"bead_id"`
	Rig          string `json:"rig"`
	Count        int    `json:"count"`
	MaxCount     int    `json:"max_count"`
	LastRespawn  string `json:"last_respawn"`
	Blocked      int    `json:"blocked"`
	UpdatedAt    string `json:"updated_at"`
	EvidenceJSON string `json:"evidence_json"`
}

type redispatchRow struct {
	BeadID          string `json:"bead_id"`
	SourceRig       string `json:"source_rig"`
	TargetRig       string `json:"target_rig"`
	AttemptCount    int    `json:"attempt_count"`
	LastAttemptTime string `json:"last_attempt_time"`
	CooldownUntil   string `json:"cooldown_until"`
	Escalated       int    `json:"escalated"`
	EscalatedAt     string `json:"escalated_at"`
	LastAction      string `json:"last_action"`
	UpdatedAt       string `json:"updated_at"`
	EvidenceJSON    string `json:"evidence_json"`
}

type cleanupRow struct {
	CleanupID    string `json:"cleanup_id"`
	Rig          string `json:"rig"`
	PolecatName  string `json:"polecat_name"`
	BeadID       string `json:"bead_id"`
	Session      string `json:"session"`
	Status       string `json:"status"`
	Blocker      string `json:"blocker"`
	WispID       string `json:"wisp_id"`
	AttemptCount int    `json:"attempt_count"`
	LastError    string `json:"last_error"`
	UpdatedAt    string `json:"updated_at"`
	PayloadJSON  string `json:"payload_json"`
}

type dependencyRow struct {
	DependencyKey string `json:"dependency_key"`
	Name          string `json:"name"`
	Scope         string `json:"scope"`
	Status        string `json:"status"`
	Detail        string `json:"detail"`
	CheckedAt     string `json:"checked_at"`
	LastHealthyAt string `json:"last_healthy_at"`
	PayloadJSON   string `json:"payload_json"`
}

func decodeLeaseRows(rows []leaseRow) ([]LeaseRecord, error) {
	out := make([]LeaseRecord, 0, len(rows))
	for _, row := range rows {
		evidence, err := unmarshalJSONMap(row.EvidenceJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, LeaseRecord{
			LeaseID:    row.LeaseID,
			Service:    row.Service,
			Rig:        row.Rig,
			Session:    row.Session,
			Holder:     row.Holder,
			Status:     row.Status,
			AcquiredAt: row.AcquiredAt,
			RenewedAt:  row.RenewedAt,
			ReleasedAt: row.ReleasedAt,
			Detail:     row.Detail,
			Evidence:   evidence,
		})
	}
	return out, nil
}

func decodeRespawnRows(rows []respawnRow) ([]RespawnCounter, error) {
	out := make([]RespawnCounter, 0, len(rows))
	for _, row := range rows {
		evidence, err := unmarshalJSONMap(row.EvidenceJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, RespawnCounter{
			BeadID:      row.BeadID,
			Rig:         row.Rig,
			Count:       row.Count,
			MaxCount:    row.MaxCount,
			LastRespawn: row.LastRespawn,
			Blocked:     row.Blocked != 0,
			UpdatedAt:   row.UpdatedAt,
			Evidence:    evidence,
		})
	}
	return out, nil
}

func decodeRedispatchRows(rows []redispatchRow) ([]RedispatchRecord, error) {
	out := make([]RedispatchRecord, 0, len(rows))
	for _, row := range rows {
		evidence, err := unmarshalJSONMap(row.EvidenceJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, RedispatchRecord{
			BeadID:          row.BeadID,
			SourceRig:       row.SourceRig,
			TargetRig:       row.TargetRig,
			AttemptCount:    row.AttemptCount,
			LastAttemptTime: row.LastAttemptTime,
			CooldownUntil:   row.CooldownUntil,
			Escalated:       row.Escalated != 0,
			EscalatedAt:     row.EscalatedAt,
			LastAction:      row.LastAction,
			UpdatedAt:       row.UpdatedAt,
			Evidence:        evidence,
		})
	}
	return out, nil
}

func decodeCleanupRows(rows []cleanupRow) ([]CleanupState, error) {
	out := make([]CleanupState, 0, len(rows))
	for _, row := range rows {
		payload, err := unmarshalJSONMap(row.PayloadJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, CleanupState{
			CleanupID:    row.CleanupID,
			Rig:          row.Rig,
			PolecatName:  row.PolecatName,
			BeadID:       row.BeadID,
			Session:      row.Session,
			Status:       row.Status,
			Blocker:      row.Blocker,
			WispID:       row.WispID,
			AttemptCount: row.AttemptCount,
			LastError:    row.LastError,
			UpdatedAt:    row.UpdatedAt,
			Payload:      payload,
		})
	}
	return out, nil
}

func decodeDependencyRows(rows []dependencyRow) ([]DependencyHealth, error) {
	out := make([]DependencyHealth, 0, len(rows))
	for _, row := range rows {
		payload, err := unmarshalJSONMap(row.PayloadJSON)
		if err != nil {
			return nil, err
		}
		out = append(out, DependencyHealth{
			DependencyKey: row.DependencyKey,
			Name:          row.Name,
			Scope:         row.Scope,
			Status:        row.Status,
			Detail:        row.Detail,
			CheckedAt:     row.CheckedAt,
			LastHealthyAt: row.LastHealthyAt,
			Payload:       payload,
		})
	}
	return out, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func asMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{"value": fmt.Sprint(value)}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{"value": string(data)}
	}
	return out
}

func hasMeaningfulValue(value string) bool {
	return strings.TrimSpace(value) != ""
}
