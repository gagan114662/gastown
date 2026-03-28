package witness

import gtHealth "github.com/steveyegge/gastown/internal/health"

// FilterProblemsForRig keeps only the problem agents owned by a witness's rig.
func FilterProblemsForRig(problems []*gtHealth.ProblemAgent, rigName string) []*gtHealth.ProblemAgent {
	if rigName == "" {
		return nil
	}
	filtered := make([]*gtHealth.ProblemAgent, 0, len(problems))
	for _, problem := range problems {
		if problem == nil || problem.Rig != rigName || !problem.NeedsAttention() {
			continue
		}
		filtered = append(filtered, problem)
	}
	return filtered
}
