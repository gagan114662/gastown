package health

import (
	"sort"
	"strconv"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// AgentDataSource provides structured data for agent health detection.
type AgentDataSource interface {
	ListAgentBeads() (map[string]*beads.Issue, error)
	IsSessionAlive(sessionName string) (bool, error)
}

// AgentState represents the possible states for a Gas Town agent.
type AgentState int

const (
	StateGUPPViolation AgentState = iota
	StateStalled
	StateWorking
	StateIdle
	StateZombie
)

func (s AgentState) String() string {
	switch s {
	case StateGUPPViolation:
		return "gupp"
	case StateStalled:
		return "stalled"
	case StateWorking:
		return "working"
	case StateIdle:
		return "idle"
	case StateZombie:
		return "zombie"
	default:
		return "unknown"
	}
}

func (s AgentState) Priority() int {
	return int(s)
}

func (s AgentState) NeedsAttention() bool {
	switch s {
	case StateGUPPViolation, StateStalled, StateZombie:
		return true
	default:
		return false
	}
}

func (s AgentState) Symbol() string {
	switch s {
	case StateGUPPViolation:
		return "🔥"
	case StateStalled:
		return "⚠"
	case StateWorking:
		return "●"
	case StateIdle:
		return "○"
	case StateZombie:
		return "💀"
	default:
		return "?"
	}
}

func (s AgentState) Label() string {
	switch s {
	case StateGUPPViolation:
		return "GUPP!"
	case StateStalled:
		return "STALL"
	case StateWorking:
		return "work"
	case StateIdle:
		return "idle"
	case StateZombie:
		return "dead"
	default:
		return "???"
	}
}

var GUPPViolationMinutes = int(constants.GUPPViolationTimeout.Minutes())

const StalledThresholdMinutes = 15

// ProblemAgent represents an agent that needs attention.
type ProblemAgent struct {
	Name          string     `json:"name"`
	SessionID     string     `json:"session_id"`
	Role          string     `json:"role"`
	Rig           string     `json:"rig"`
	State         AgentState `json:"state"`
	IdleMinutes   int        `json:"idle_minutes"`
	LastActivity  time.Time  `json:"last_activity,omitempty"`
	ActionHint    string     `json:"action_hint,omitempty"`
	CurrentBeadID string     `json:"current_bead_id,omitempty"`
	HasHookedWork bool       `json:"has_hooked_work"`
}

func (p *ProblemAgent) NeedsAttention() bool {
	return p.State.NeedsAttention()
}

func (p *ProblemAgent) DurationDisplay() string {
	mins := p.IdleMinutes
	if mins < 1 {
		return "<1m"
	}
	if mins < 60 {
		return strconv.Itoa(mins) + "m"
	}
	hours := mins / 60
	remaining := mins % 60
	if remaining == 0 {
		return strconv.Itoa(hours) + "h"
	}
	return strconv.Itoa(hours) + "h" + strconv.Itoa(remaining) + "m"
}

// AgentDetector analyzes agent health using structured beads data.
type AgentDetector struct {
	source AgentDataSource
}

func NewAgentDetector(bd *beads.Beads) *AgentDetector {
	return NewAgentDetectorWithSource(&defaultAgentSource{
		bd:   bd,
		tmux: tmux.NewTmux(),
	})
}

func NewAgentDetectorWithSource(source AgentDataSource) *AgentDetector {
	return &AgentDetector{source: source}
}

func (d *AgentDetector) CheckAll() ([]*ProblemAgent, error) {
	agentBeads, err := d.source.ListAgentBeads()
	if err != nil {
		return nil, err
	}

	var agents []*ProblemAgent
	for id, issue := range agentBeads {
		agent := d.analyzeAgent(id, issue)
		if agent != nil {
			agents = append(agents, agent)
		}
	}

	sort.Slice(agents, func(i, j int) bool {
		if agents[i].State.Priority() != agents[j].State.Priority() {
			return agents[i].State.Priority() < agents[j].State.Priority()
		}
		return agents[i].IdleMinutes > agents[j].IdleMinutes
	})
	return agents, nil
}

func (d *AgentDetector) analyzeAgent(id string, issue *beads.Issue) *ProblemAgent {
	rig, role, name, ok := beads.ParseAgentBeadID(id)
	if !ok {
		return nil
	}

	displayName := name
	if displayName == "" {
		displayName = role
	}

	sessionName := DeriveSessionName(rig, role, name)
	agent := &ProblemAgent{
		Name:          displayName,
		SessionID:     sessionName,
		Role:          role,
		Rig:           rig,
		CurrentBeadID: id,
		HasHookedWork: issue.HookBead != "",
	}

	updatedAt, err := time.Parse(time.RFC3339, issue.UpdatedAt)
	if err != nil {
		updatedAt, err = time.Parse("2006-01-02T15:04:05", issue.UpdatedAt)
	}
	if err == nil {
		agent.LastActivity = updatedAt
		agent.IdleMinutes = int(time.Since(updatedAt).Minutes())
	}

	alive, err := d.source.IsSessionAlive(sessionName)
	if err == nil && !alive {
		agent.State = StateZombie
		agent.ActionHint = "Session dead - may need restart"
		return agent
	}

	hasHook := issue.HookBead != ""
	stalledThreshold := StalledThresholdMinutes
	guppThreshold := GUPPViolationMinutes
	if hasHook && IsRalphMode(issue) {
		stalledThreshold = 120
		guppThreshold = 240
	}

	if hasHook && agent.IdleMinutes >= guppThreshold {
		agent.State = StateGUPPViolation
		agent.ActionHint = "GUPP violation: hooked work + " + strconv.Itoa(agent.IdleMinutes) + "m no progress"
		return agent
	}

	if hasHook && agent.IdleMinutes >= stalledThreshold {
		agent.State = StateStalled
		agent.ActionHint = "No progress for " + strconv.Itoa(agent.IdleMinutes) + "m"
		return agent
	}

	if hasHook {
		agent.State = StateWorking
	} else {
		agent.State = StateIdle
	}
	return agent
}

func IsGUPPViolation(hasHookedWork bool, minutesSinceProgress int) bool {
	return hasHookedWork && minutesSinceProgress >= GUPPViolationMinutes
}

func IsRalphMode(issue *beads.Issue) bool {
	if issue == nil || issue.Description == "" {
		return false
	}
	fields := beads.ParseAgentFields(issue.Description)
	return fields != nil && fields.Mode == "ralph"
}

func DeriveSessionName(rig, role, name string) string {
	switch role {
	case constants.RoleMayor:
		return session.MayorSessionName()
	case constants.RoleDeacon:
		return session.DeaconSessionName()
	case constants.RoleWitness:
		return session.WitnessSessionName(session.PrefixFor(rig))
	case constants.RoleRefinery:
		return session.RefinerySessionName(session.PrefixFor(rig))
	case constants.RoleCrew:
		return session.CrewSessionName(session.PrefixFor(rig), name)
	case constants.RolePolecat:
		return session.PolecatSessionName(session.PrefixFor(rig), name)
	default:
		rigPrefix := session.PrefixFor(rig)
		if rig == "" {
			return session.HQPrefix + role
		}
		if name == "" {
			return rigPrefix + "-" + role
		}
		return rigPrefix + "-" + role + "-" + name
	}
}

type defaultAgentSource struct {
	bd   *beads.Beads
	tmux *tmux.Tmux
}

func (s *defaultAgentSource) ListAgentBeads() (map[string]*beads.Issue, error) {
	return s.bd.ListAgentBeads()
}

func (s *defaultAgentSource) IsSessionAlive(sessionName string) (bool, error) {
	status := s.tmux.CheckSessionHealth(sessionName, 0)
	return status == tmux.SessionHealthy, nil
}
