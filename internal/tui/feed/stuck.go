package feed

import (
	"github.com/steveyegge/gastown/internal/beads"
	gtHealth "github.com/steveyegge/gastown/internal/health"
)

type HealthDataSource = gtHealth.AgentDataSource
type AgentState = gtHealth.AgentState

const (
	StateGUPPViolation      = gtHealth.StateGUPPViolation
	StateStalled            = gtHealth.StateStalled
	StateWorking            = gtHealth.StateWorking
	StateIdle               = gtHealth.StateIdle
	StateZombie             = gtHealth.StateZombie
	StalledThresholdMinutes = gtHealth.StalledThresholdMinutes
)

var GUPPViolationMinutes = gtHealth.GUPPViolationMinutes

type ProblemAgent = gtHealth.ProblemAgent
type StuckDetector = gtHealth.AgentDetector

func NewStuckDetector(bd *beads.Beads) *StuckDetector {
	return gtHealth.NewAgentDetector(bd)
}

func NewStuckDetectorWithSource(source HealthDataSource) *StuckDetector {
	return gtHealth.NewAgentDetectorWithSource(source)
}

func IsGUPPViolation(hasHookedWork bool, minutesSinceProgress int) bool {
	return gtHealth.IsGUPPViolation(hasHookedWork, minutesSinceProgress)
}

func deriveSessionName(rig, role, name string) string {
	return gtHealth.DeriveSessionName(rig, role, name)
}

func isRalphMode(issue *beads.Issue) bool {
	return gtHealth.IsRalphMode(issue)
}
