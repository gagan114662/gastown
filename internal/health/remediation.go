package health

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

type RemediationAction string

const (
	ActionNone        RemediationAction = "none"
	ActionNudge       RemediationAction = "nudge"
	ActionHandoff     RemediationAction = "handoff"
	ActionRestart     RemediationAction = "restart"
	ActionEscalate    RemediationAction = "escalate"
	ActionInvestigate RemediationAction = "investigate"
)

type RemediationDecision struct {
	AgentID     string            `json:"agent_id,omitempty"`
	SessionID   string            `json:"session_id,omitempty"`
	Rig         string            `json:"rig,omitempty"`
	State       AgentState        `json:"state"`
	Action      RemediationAction `json:"action"`
	Reason      string            `json:"reason"`
	RepeatCount int               `json:"repeat_count,omitempty"`
}

type RemediationRecord struct {
	AgentID      string    `json:"agent_id"`
	LastState    string    `json:"last_state,omitempty"`
	RepeatCount  int       `json:"repeat_count"`
	LastAction   string    `json:"last_action,omitempty"`
	LastActionAt time.Time `json:"last_action_at,omitempty"`
	LastSeenAt   time.Time `json:"last_seen_at,omitempty"`
}

type RemediationState struct {
	Agents      map[string]*RemediationRecord `json:"agents"`
	LastUpdated time.Time                     `json:"last_updated"`
}

func RemediationStateFile(townRoot string) string {
	return filepath.Join(townRoot, "deacon", "auto-remediation-state.json")
}

func LoadRemediationState(townRoot string) (*RemediationState, error) {
	path := RemediationStateFile(townRoot)
	data, err := os.ReadFile(path) //nolint:gosec // derived from workspace root
	if err != nil {
		if os.IsNotExist(err) {
			return &RemediationState{Agents: map[string]*RemediationRecord{}}, nil
		}
		return nil, err
	}
	var state RemediationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Agents == nil {
		state.Agents = map[string]*RemediationRecord{}
	}
	return &state, nil
}

func SaveRemediationState(townRoot string, state *RemediationState) error {
	if state == nil {
		return nil
	}
	state.LastUpdated = time.Now().UTC()
	path := RemediationStateFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (s *RemediationState) Observe(agent *ProblemAgent) *RemediationRecord {
	if s.Agents == nil {
		s.Agents = map[string]*RemediationRecord{}
	}
	record, ok := s.Agents[agent.CurrentBeadID]
	if !ok {
		record = &RemediationRecord{AgentID: agent.CurrentBeadID}
		s.Agents[agent.CurrentBeadID] = record
	}
	if record.LastState == agent.State.String() {
		record.RepeatCount++
	} else {
		record.LastState = agent.State.String()
		record.RepeatCount = 1
	}
	record.LastSeenAt = time.Now().UTC()
	return record
}

func (s *RemediationState) RecordAction(agent *ProblemAgent, action RemediationAction) {
	if s.Agents == nil {
		s.Agents = map[string]*RemediationRecord{}
	}
	record, ok := s.Agents[agent.CurrentBeadID]
	if !ok {
		record = &RemediationRecord{
			AgentID:     agent.CurrentBeadID,
			LastState:   agent.State.String(),
			RepeatCount: 1,
			LastSeenAt:  time.Now().UTC(),
		}
		s.Agents[agent.CurrentBeadID] = record
	}
	record.LastAction = string(action)
	record.LastActionAt = time.Now().UTC()
}

func DecideRemediation(agent *ProblemAgent, record *RemediationRecord) RemediationDecision {
	decision := RemediationDecision{
		AgentID:     agent.CurrentBeadID,
		SessionID:   agent.SessionID,
		Rig:         agent.Rig,
		State:       agent.State,
		Action:      ActionNone,
		RepeatCount: 1,
	}
	if record != nil && record.RepeatCount > 0 {
		decision.RepeatCount = record.RepeatCount
	}

	switch agent.State {
	case StateStalled:
		if decision.RepeatCount > 1 {
			decision.Action = ActionHandoff
			decision.Reason = "stalled twice; refresh session context"
		} else {
			decision.Action = ActionNudge
			decision.Reason = "stalled once; nudge before cycling"
		}
	case StateGUPPViolation:
		if decision.RepeatCount > 1 {
			decision.Action = ActionEscalate
			decision.Reason = "repeated GUPP violation after previous intervention"
		} else {
			decision.Action = ActionHandoff
			decision.Reason = "hooked work has stalled past GUPP threshold"
		}
	case StateZombie:
		decision.Action = ActionRestart
		decision.Reason = "dead session with tracked work requires restart"
	default:
		decision.Reason = "no remediation required"
	}
	return decision
}

type AnomalySummary struct {
	Rig           string            `json:"rig,omitempty"`
	WindowMinutes int               `json:"window_minutes"`
	ActivityCount int               `json:"activity_count"`
	DoneCount     int               `json:"done_count"`
	Types         map[string]int    `json:"types,omitempty"`
	Action        RemediationAction `json:"action,omitempty"`
	Reason        string            `json:"reason,omitempty"`
}

// DetectEventAnomaly looks for high activity windows without completions.
func DetectEventAnomaly(townRoot, rig string, window time.Duration, minActivity int) (*AnomalySummary, error) {
	if window <= 0 {
		window = 15 * time.Minute
	}
	if minActivity <= 0 {
		minActivity = 12
	}
	path := filepath.Join(townRoot, events.EventsFile)
	file, err := os.Open(path) //nolint:gosec // derived from workspace root
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	cutoff := time.Now().Add(-window)
	summary := &AnomalySummary{
		Rig:           rig,
		WindowMinutes: int(window.Minutes()),
		Types:         map[string]int{},
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event events.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil || ts.Before(cutoff) {
			continue
		}
		if !eventMatchesRig(event, rig) {
			continue
		}
		summary.ActivityCount++
		summary.Types[event.Type]++
		if event.Type == events.TypeDone {
			summary.DoneCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if summary.ActivityCount >= minActivity && summary.DoneCount == 0 {
		summary.Action = ActionInvestigate
		summary.Reason = fmt.Sprintf("%d recent events without any done completions", summary.ActivityCount)
		return summary, nil
	}
	return nil, nil
}

func eventMatchesRig(event events.Event, rig string) bool {
	if strings.TrimSpace(rig) == "" {
		return true
	}
	if strings.Contains(event.Actor, rig+"/") {
		return true
	}
	if value, ok := event.Payload["rig"].(string); ok && value == rig {
		return true
	}
	return false
}
