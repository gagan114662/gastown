package operatorview

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/checkpoint"
	"github.com/steveyegge/gastown/internal/controlplane"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// LoadTownSnapshot returns a reconciled, read-only view of the town.
func LoadTownSnapshot(townRoot string) (*TownSnapshot, error) {
	_ = session.InitRegistry(townRoot)

	store, err := controlplane.Open(townRoot)
	if err != nil {
		return nil, err
	}

	runtimeRecords, err := store.ListAgentRuntime()
	if err != nil {
		return nil, err
	}
	recentEvents, err := store.ListEvents(50)
	if err != nil {
		return nil, err
	}
	incidents, err := store.ListIncidents(25)
	if err != nil {
		return nil, err
	}

	t := tmux.NewTmux()
	tmuxSessions, err := t.ListSessions()
	if err != nil {
		return nil, err
	}

	sessionSet := make(map[string]struct{}, len(tmuxSessions))
	for _, name := range tmuxSessions {
		sessionSet[name] = struct{}{}
	}

	snapshots := make([]AgentSnapshot, 0, len(runtimeRecords)+len(tmuxSessions))
	seen := make(map[string]struct{}, len(runtimeRecords)+len(tmuxSessions))
	for i := range runtimeRecords {
		record := runtimeRecords[i]
		snapshot := buildAgentSnapshot(townRoot, store, t, sessionSet, &record)
		snapshots = append(snapshots, snapshot)
		seen[snapshot.AgentID] = struct{}{}
		if snapshot.Session != "" {
			seen[snapshot.Session] = struct{}{}
		}
	}

	for _, sessionName := range tmuxSessions {
		if _, ok := seen[sessionName]; ok {
			continue
		}
		record := runtimeRecordFromSession(t, sessionName)
		snapshot := buildAgentSnapshot(townRoot, store, t, sessionSet, &record)
		snapshots = append(snapshots, snapshot)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Rig != snapshots[j].Rig {
			return snapshots[i].Rig < snapshots[j].Rig
		}
		if snapshots[i].Role != snapshots[j].Role {
			return snapshots[i].Role < snapshots[j].Role
		}
		return snapshots[i].Session < snapshots[j].Session
	})

	townConflicts := make([]string, 0)
	conflictCount := 0
	for _, snapshot := range snapshots {
		conflictCount += len(snapshot.Conflicts)
	}
	if conflictCount > 0 {
		townConflicts = append(townConflicts, fmt.Sprintf("%d agent conflict(s) detected", conflictCount))
	}
	if len(incidents) > 0 {
		townConflicts = append(townConflicts, fmt.Sprintf("%d incident candidate(s) in recent events", len(incidents)))
	}

	status := "healthy"
	statusReason := "runtime sources agree"
	sourceAgreement := "agreeing"
	if len(townConflicts) > 0 {
		status = "degraded"
		statusReason = townConflicts[0]
		sourceAgreement = "conflict"
	}

	return &TownSnapshot{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		TownRoot:        townRoot,
		Status:          status,
		StatusReason:    statusReason,
		SourceAgreement: sourceAgreement,
		Conflicts:       townConflicts,
		Projections: []ProjectionStatus{
			{Name: "controlplane", Status: "ok", Detail: controlplane.DBPath(townRoot)},
			{Name: "tmux", Status: projectionStatus(len(tmuxSessions) > 0, "sessions discovered", "no live sessions")},
			{Name: "events", Status: projectionStatus(len(recentEvents) > 0, "recent events available", "no recent events")},
			{Name: "incidents", Status: projectionStatus(len(incidents) > 0, "incident candidates present", "no incident candidates")},
		},
		Agents:       snapshots,
		RecentEvents: recentEvents,
		Incidents:    incidents,
	}, nil
}

// LoadAgentSnapshot returns one reconciled agent view.
func LoadAgentSnapshot(townRoot, agentID string) (*AgentSnapshot, error) {
	snapshot, err := LoadTownSnapshot(townRoot)
	if err != nil {
		return nil, err
	}
	for i := range snapshot.Agents {
		agent := snapshot.Agents[i]
		if agent.AgentID == agentID || agent.Session == agentID || agent.Beads.AgentBeadID == agentID {
			return &agent, nil
		}
	}
	return nil, nil
}

func buildAgentSnapshot(townRoot string, store *controlplane.Store, t *tmux.Tmux, sessionSet map[string]struct{}, record *controlplane.AgentRuntimeRecord) AgentSnapshot {
	snapshot := AgentSnapshot{
		Status:          "unknown",
		StatusReason:    "no evidence collected",
		SourceAgreement: "partial",
	}
	if record != nil {
		snapshot.AgentID = record.AgentID
		snapshot.Role = record.Role
		snapshot.Rig = record.Rig
		snapshot.AgentName = record.AgentName
		snapshot.Session = record.Session
		snapshot.RunID = record.RunID
		snapshot.WorkDir = record.WorkDir
		snapshot.Status = record.Status
		snapshot.StatusReason = record.StatusReason
		snapshot.SourceAgreement = record.SourceAgreement
		snapshot.Runtime = record
	}
	if snapshot.AgentID == "" {
		snapshot.AgentID = snapshot.Session
	}

	if snapshot.Session != "" {
		if _, ok := sessionSet[snapshot.Session]; ok {
			snapshot.Tmux.Exists = true
			snapshot.Tmux.Session = snapshot.Session
			if workDir, err := t.GetPaneWorkDir(snapshot.Session); err == nil {
				snapshot.Tmux.WorkDir = workDir
				if snapshot.WorkDir == "" {
					snapshot.WorkDir = workDir
				}
			}
			if runID, err := t.GetEnvironment(snapshot.Session, "GT_RUN"); err == nil {
				snapshot.Tmux.RunID = runID
				if snapshot.RunID == "" {
					snapshot.RunID = runID
				}
			}
			if role, err := t.GetEnvironment(snapshot.Session, "GT_ROLE"); err == nil && snapshot.Role == "" {
				snapshot.Role = role
				snapshot.Tmux.Role = role
			}
			if rig, err := t.GetEnvironment(snapshot.Session, "GT_RIG"); err == nil && snapshot.Rig == "" {
				snapshot.Rig = rig
				snapshot.Tmux.Rig = rig
			}
			if name, err := t.GetEnvironment(snapshot.Session, "GT_AGENT_NAME"); err == nil && snapshot.AgentName == "" {
				snapshot.AgentName = name
				snapshot.Tmux.Agent = name
			}
		}
	}

	if expectsHeartbeat(snapshot.Role) && snapshot.Session != "" {
		if hb := polecat.ReadSessionHeartbeat(townRoot, snapshot.Session); hb != nil {
			snapshot.Heartbeat.Exists = true
			snapshot.Heartbeat.Timestamp = hb.Timestamp.UTC().Format(time.RFC3339)
			snapshot.Heartbeat.State = string(hb.EffectiveState())
			snapshot.Heartbeat.Context = hb.Context
			snapshot.Heartbeat.Bead = hb.Bead
			snapshot.Heartbeat.Fresh = time.Since(hb.Timestamp) < polecat.SessionHeartbeatStaleThreshold
		}
	}

	if snapshot.WorkDir != "" {
		if cp, err := checkpoint.Read(snapshot.WorkDir); err == nil && cp != nil {
			snapshot.Checkpoint.Exists = true
			snapshot.Checkpoint.Path = checkpoint.Path(snapshot.WorkDir)
			snapshot.Checkpoint.Timestamp = cp.Timestamp.UTC().Format(time.RFC3339)
			snapshot.Checkpoint.SessionID = cp.SessionID
			snapshot.Checkpoint.HookedBead = cp.HookedBead
			snapshot.Checkpoint.CurrentStep = cp.CurrentStep
			snapshot.Checkpoint.Summary = cp.Summary()
		}
	}

	if beadID := agentBeadID(snapshot.Role, snapshot.Rig, snapshot.AgentName); beadID != "" {
		snapshot.Beads.AgentBeadID = beadID
		if issue, fields, err := beads.New(townRoot).GetAgentBead(beadID); err == nil && issue != nil && fields != nil {
			snapshot.Beads.Exists = true
			snapshot.Beads.AgentState = fields.AgentState
			snapshot.Beads.HookBead = fields.HookBead
			snapshot.Beads.ActiveMR = fields.ActiveMR
			snapshot.Beads.CleanupStatus = fields.CleanupStatus
			snapshot.Beads.CompletionTime = fields.CompletionTime
			snapshot.Beads.NotificationLvl = fields.NotificationLevel
		}
	}

	if snapshot.Session != "" {
		pidPath := filepath.Join(townRoot, ".runtime", "pids", snapshot.Session+".pid")
		if _, err := os.Stat(pidPath); err == nil {
			snapshot.PID.Exists = true
			snapshot.PID.Path = pidPath
		}
		if events, err := store.ListEventsBySession(snapshot.Session, 10); err == nil {
			snapshot.RecentEvents = events
			snapshot.Decisions = supervisorDecisions(events)
		}
	}

	reconcileSnapshot(&snapshot)
	return snapshot
}

func runtimeRecordFromSession(t *tmux.Tmux, sessionName string) controlplane.AgentRuntimeRecord {
	record := controlplane.AgentRuntimeRecord{
		AgentID:         sessionName,
		Session:         sessionName,
		Status:          "running",
		StatusReason:    "tmux session exists",
		SourceAgreement: "tmux-only",
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if identity, err := session.ParseSessionName(sessionName); err == nil {
		record.Role = string(identity.Role)
		record.Rig = identity.Rig
		record.AgentName = identity.Name
	}
	if workDir, err := t.GetPaneWorkDir(sessionName); err == nil {
		record.WorkDir = workDir
	}
	if runID, err := t.GetEnvironment(sessionName, "GT_RUN"); err == nil {
		record.RunID = runID
	}
	return record
}

func reconcileSnapshot(snapshot *AgentSnapshot) {
	conflicts := make([]string, 0)
	projections := make([]ProjectionStatus, 0, 6)

	projections = append(projections, ProjectionStatus{
		Name:      "controlplane",
		Status:    projectionStatus(snapshot.Runtime != nil, "present", "absent"),
		Detail:    snapshot.StatusReason,
		UpdatedAt: runtimeUpdatedAt(snapshot.Runtime),
	})

	if snapshot.Tmux.Exists {
		projections = append(projections, ProjectionStatus{Name: "tmux", Status: "ok", Detail: snapshot.Tmux.Session})
	} else {
		projections = append(projections, ProjectionStatus{Name: "tmux", Status: "missing", Detail: "session not found"})
		if snapshot.Runtime != nil && snapshot.Runtime.Status == "running" {
			conflicts = append(conflicts, "control-plane says running but tmux session is missing")
		}
	}

	if expectsHeartbeat(snapshot.Role) {
		switch {
		case snapshot.Heartbeat.Exists && snapshot.Heartbeat.Fresh:
			projections = append(projections, ProjectionStatus{Name: "heartbeat", Status: "ok", Detail: snapshot.Heartbeat.State, UpdatedAt: snapshot.Heartbeat.Timestamp})
		case snapshot.Heartbeat.Exists && !snapshot.Heartbeat.Fresh:
			projections = append(projections, ProjectionStatus{Name: "heartbeat", Status: "stale", Detail: snapshot.Heartbeat.State, UpdatedAt: snapshot.Heartbeat.Timestamp})
			conflicts = append(conflicts, "heartbeat is stale")
		default:
			projections = append(projections, ProjectionStatus{Name: "heartbeat", Status: "missing", Detail: "no heartbeat file"})
			if snapshot.Tmux.Exists {
				conflicts = append(conflicts, "live session has no heartbeat")
			}
		}
	}

	if snapshot.Checkpoint.Exists {
		projections = append(projections, ProjectionStatus{Name: "checkpoint", Status: "ok", Detail: snapshot.Checkpoint.Summary, UpdatedAt: snapshot.Checkpoint.Timestamp})
		if !snapshot.Tmux.Exists {
			conflicts = append(conflicts, "checkpoint exists but session is missing")
		}
	} else if snapshot.WorkDir != "" {
		projections = append(projections, ProjectionStatus{Name: "checkpoint", Status: "missing", Detail: "no checkpoint"})
	}

	if snapshot.Beads.AgentBeadID != "" {
		if snapshot.Beads.Exists {
			projections = append(projections, ProjectionStatus{Name: "beads", Status: "ok", Detail: snapshot.Beads.AgentState})
			if !beadsStateMatches(snapshot.Status, snapshot.Beads.AgentState) {
				conflicts = append(conflicts, fmt.Sprintf("beads state %q disagrees with runtime status %q", snapshot.Beads.AgentState, snapshot.Status))
			}
		} else {
			projections = append(projections, ProjectionStatus{Name: "beads", Status: "missing", Detail: snapshot.Beads.AgentBeadID})
		}
	}

	if snapshot.PID.Exists {
		projections = append(projections, ProjectionStatus{Name: "pid_tracking", Status: "ok", Detail: snapshot.PID.Path})
	} else if snapshot.Session != "" {
		projections = append(projections, ProjectionStatus{Name: "pid_tracking", Status: "missing", Detail: "no pid file"})
	}

	snapshot.Conflicts = conflicts
	snapshot.Projections = projections

	switch {
	case snapshot.Tmux.Exists && snapshot.Heartbeat.Exists && !snapshot.Heartbeat.Fresh:
		snapshot.Status = "degraded"
		snapshot.StatusReason = "session exists but heartbeat is stale"
	case snapshot.Tmux.Exists && snapshot.Status == "":
		snapshot.Status = "running"
		snapshot.StatusReason = "session exists"
	case !snapshot.Tmux.Exists && snapshot.Checkpoint.Exists:
		snapshot.Status = "recoverable"
		snapshot.StatusReason = "checkpoint available but session is missing"
	case snapshot.Status == "":
		snapshot.Status = "unknown"
		snapshot.StatusReason = "no runtime record"
	}

	switch {
	case len(conflicts) == 0 && snapshot.Runtime != nil && snapshot.Tmux.Exists:
		snapshot.SourceAgreement = "agreeing"
	case len(conflicts) > 0:
		snapshot.SourceAgreement = "conflict"
	default:
		snapshot.SourceAgreement = "partial"
	}
}

func supervisorDecisions(events []controlplane.TownEvent) []SupervisorDecision {
	out := make([]SupervisorDecision, 0, len(events))
	for _, event := range events {
		if !isSupervisorDecision(event.Kind) {
			continue
		}
		out = append(out, SupervisorDecision{
			Timestamp: event.Timestamp,
			Kind:      event.Kind,
			Actor:     event.Actor,
			Role:      event.Role,
			Rig:       event.Rig,
			Session:   event.Session,
			Outcome:   event.Outcome,
			Reason:    event.Reason,
		})
	}
	return out
}

func isSupervisorDecision(kind string) bool {
	switch kind {
	case "patrol_started", "polecat_checked", "polecat_nudged", "escalation_sent", "escalation_closed", "session_death", "scheduler_dispatch_failed", "scheduler_close_retry":
		return true
	default:
		return false
	}
}

func expectsHeartbeat(role string) bool {
	switch role {
	case "polecat", "crew", "dog":
		return true
	default:
		return false
	}
}

func beadsStateMatches(runtimeStatus, beadsState string) bool {
	if runtimeStatus == "" || beadsState == "" {
		return true
	}
	switch runtimeStatus {
	case "running", "working", "degraded":
		return beadsState == "running" || beadsState == "working" || beadsState == "idle" || beadsState == "spawning"
	case "stopped", "recoverable":
		return beadsState == "done" || beadsState == "nuked" || beadsState == "stuck" || beadsState == "escalated"
	default:
		return true
	}
}

func projectionStatus(ok bool, okDetail, badDetail string) string {
	if ok {
		return "ok"
	}
	if okDetail == "" && badDetail == "" {
		return "unknown"
	}
	return "missing"
}

func runtimeUpdatedAt(record *controlplane.AgentRuntimeRecord) string {
	if record == nil {
		return ""
	}
	return record.UpdatedAt
}

func agentBeadID(role, rig, agentName string) string {
	if rig == "" || role == "" {
		return ""
	}
	return beads.AgentBeadIDWithPrefix(session.PrefixFor(rig), rig, role, agentName)
}

func normalizeRole(role string) string {
	return strings.TrimSpace(strings.ToLower(role))
}
