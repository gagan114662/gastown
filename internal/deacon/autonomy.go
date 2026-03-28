package deacon

import gtHealth "github.com/steveyegge/gastown/internal/health"

// DecideRemediation forwards deacon patrol decisions through the shared health package.
func DecideRemediation(agent *gtHealth.ProblemAgent, record *gtHealth.RemediationRecord) gtHealth.RemediationDecision {
	return gtHealth.DecideRemediation(agent, record)
}
