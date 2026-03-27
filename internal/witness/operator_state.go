package witness

import (
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/controlplane"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
)

func recordCleanupStateTransition(townRoot, rigName, polecatName, beadID, status, blocker, wispID string, transitionErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	sessionID := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
	attemptCount := 1
	if store, err := controlplane.Open(townRoot); err == nil {
		if existing, getErr := store.GetCleanupStateByPolecat(rigName, polecatName); getErr == nil && existing != nil {
			attemptCount = existing.AttemptCount + 1
		}
		_ = store.UpsertCleanupState(controlplane.CleanupState{
			CleanupID:    controlplane.CleanupKey(rigName, polecatName),
			Rig:          rigName,
			PolecatName:  polecatName,
			BeadID:       beadID,
			Session:      sessionID,
			Status:       status,
			Blocker:      blocker,
			WispID:       wispID,
			AttemptCount: attemptCount,
			LastError:    errorString(transitionErr),
			UpdatedAt:    now,
			Payload: map[string]interface{}{
				"rig":          rigName,
				"polecat_name": polecatName,
				"bead_id":      beadID,
				"status":       status,
				"blocker":      blocker,
				"wisp_id":      wispID,
			},
		})
	}

	outcome := "success"
	if transitionErr != nil {
		outcome = "error"
	} else if blocker != "" {
		outcome = "deferred"
	}
	_ = events.LogEventAt(townRoot, events.Event{
		Kind:       events.TypeCleanupState,
		Type:       events.TypeCleanupState,
		Actor:      fmt.Sprintf("%s/witness", rigName),
		Role:       "witness",
		Rig:        rigName,
		Session:    sessionID,
		BeadID:     beadID,
		Outcome:    outcome,
		Reason:     firstNonEmpty(blocker, errorString(transitionErr), status),
		Visibility: events.VisibilityAudit,
		Payload: map[string]interface{}{
			"status":        status,
			"blocker":       blocker,
			"wisp_id":       wispID,
			"attempt_count": attemptCount,
		},
	})
}

func recordRespawnBlocked(townRoot, rigName, polecatName, beadID, reason string, maxRespawns int) {
	sessionID := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
	_ = events.LogEventAt(townRoot, events.Event{
		Kind:       events.TypeRespawnBlocked,
		Type:       events.TypeRespawnBlocked,
		Actor:      fmt.Sprintf("%s/witness", rigName),
		Role:       "witness",
		Rig:        rigName,
		Session:    sessionID,
		BeadID:     beadID,
		Outcome:    "deferred",
		Reason:     reason,
		Visibility: events.VisibilityAudit,
		Payload: map[string]interface{}{
			"max_respawns": maxRespawns,
			"polecat_name": polecatName,
		},
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
