package witness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/controlplane"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/lock"
	"github.com/steveyegge/gastown/internal/workspace"
)

// respawnMu serializes in-process access to the respawn state file.
// Cross-process serialization is handled by lock.FlockAcquire on a
// sibling .flock file (see RecordBeadRespawn, ShouldBlockRespawn, etc.).
var respawnMu sync.Mutex

// beadRespawnRecord tracks how many times a single bead has been reset for re-dispatch.
type beadRespawnRecord struct {
	BeadID      string    `json:"bead_id"`
	Count       int       `json:"count"`
	LastRespawn time.Time `json:"last_respawn"`
}

// beadRespawnState holds respawn counts for all tracked beads.
type beadRespawnState struct {
	Beads       map[string]*beadRespawnRecord `json:"beads"`
	LastUpdated time.Time                     `json:"last_updated"`
}

func beadRespawnStateFile(townRoot string) string {
	return filepath.Join(townRoot, "witness", "bead-respawn-counts.json")
}

func loadBeadRespawnState(townRoot string) *beadRespawnState {
	data, err := os.ReadFile(beadRespawnStateFile(townRoot)) //nolint:gosec // G304: path from trusted townRoot
	if err != nil {
		return &beadRespawnState{Beads: make(map[string]*beadRespawnRecord)}
	}
	var state beadRespawnState
	if err := json.Unmarshal(data, &state); err != nil {
		return &beadRespawnState{Beads: make(map[string]*beadRespawnRecord)}
	}
	if state.Beads == nil {
		state.Beads = make(map[string]*beadRespawnRecord)
	}
	return &state
}

func saveBeadRespawnState(townRoot string, state *beadRespawnState) error {
	stateFile := beadRespawnStateFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating witness dir: %w", err)
	}
	state.LastUpdated = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling respawn state: %w", err)
	}
	return os.WriteFile(stateFile, data, 0600)
}

// ShouldBlockRespawn returns true if the bead has already been respawned
// MaxBeadRespawns times (from operational config). When true, the caller
// should escalate to mayor instead of sending RECOVERED_BEAD to deacon
// for re-dispatch. This is the primary circuit breaker for spawn storms
// (clown show #22).
func ShouldBlockRespawn(workDir, beadID string) bool {
	respawnMu.Lock()
	defer respawnMu.Unlock()

	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}
	maxRespawns := config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV()

	// Cross-process flock to serialize with other witness instances.
	unlock, flockErr := lock.FlockAcquire(beadRespawnStateFile(townRoot) + ".flock")
	if flockErr == nil {
		defer unlock()
	}

	rec, _ := loadMergedRespawnRecord(townRoot, beadID)
	if rec == nil {
		return false
	}
	return rec.Count >= maxRespawns
}

// RecordBeadRespawn increments the respawn count for beadID and returns the new count.
// workDir is the rig path; townRoot is resolved internally via workspace.Find.
// On state file errors the count is still incremented in memory and returned, so the
// caller can log/warn without blocking the respawn itself.
//
// Serialized via respawnMu (in-process) and flock (cross-process) to prevent
// concurrent patrol cycles from racing on the load-modify-save cycle.
func RecordBeadRespawn(workDir, beadID string) int {
	respawnMu.Lock()
	defer respawnMu.Unlock()

	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}

	// Cross-process flock to serialize with other witness instances.
	unlock, flockErr := lock.FlockAcquire(beadRespawnStateFile(townRoot) + ".flock")
	if flockErr == nil {
		defer unlock()
	}

	rec, store := loadMergedRespawnRecord(townRoot, beadID)
	if rec == nil {
		rec = &beadRespawnRecord{BeadID: beadID}
	}
	rec.Count++
	rec.LastRespawn = time.Now().UTC()
	persistRespawnProjection(townRoot, rec)
	if store != nil {
		rigName := rigForBead(townRoot, beadID)
		_ = store.UpsertRespawnCounter(controlplane.RespawnCounter{
			BeadID:      beadID,
			Rig:         rigName,
			Count:       rec.Count,
			MaxCount:    config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV(),
			LastRespawn: rec.LastRespawn.Format(time.RFC3339),
			Blocked:     rec.Count >= config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV(),
			UpdatedAt:   rec.LastRespawn.Format(time.RFC3339),
			Evidence: map[string]interface{}{
				"projection_file": beadRespawnStateFile(townRoot),
				"source":          "witness",
			},
		})
	}
	_ = events.LogEventAt(townRoot, events.Event{
		Kind:       events.TypeRespawnRecorded,
		Type:       events.TypeRespawnRecorded,
		Actor:      witnessActor(townRoot, beadID),
		Role:       "witness",
		Rig:        rigForBead(townRoot, beadID),
		BeadID:     beadID,
		Outcome:    "success",
		Visibility: events.VisibilityAudit,
		Payload: map[string]interface{}{
			"count":      rec.Count,
			"max_count":  config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV(),
			"state_file": beadRespawnStateFile(townRoot),
		},
	})
	return rec.Count
}

// ResetBeadRespawnCount resets the respawn counter for beadID to zero.
// Used by `gt sling respawn-reset` to allow re-dispatch after investigation.
func ResetBeadRespawnCount(workDir, beadID string) error {
	respawnMu.Lock()
	defer respawnMu.Unlock()

	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}

	// Cross-process flock to serialize with other witness instances.
	unlock, flockErr := lock.FlockAcquire(beadRespawnStateFile(townRoot) + ".flock")
	if flockErr == nil {
		defer unlock()
	}

	state := loadBeadRespawnState(townRoot)
	delete(state.Beads, beadID)
	if err := saveBeadRespawnState(townRoot, state); err != nil {
		return err
	}
	if store, err := controlplane.Open(townRoot); err == nil {
		_ = store.DeleteRespawnCounter(beadID)
	}
	return nil
}

func loadMergedRespawnRecord(townRoot, beadID string) (*beadRespawnRecord, *controlplane.Store) {
	state := loadBeadRespawnState(townRoot)
	var fileRec *beadRespawnRecord
	if state != nil && state.Beads != nil {
		if rec, ok := state.Beads[beadID]; ok {
			copyRec := *rec
			fileRec = &copyRec
		}
	}

	store, err := controlplane.Open(townRoot)
	if err != nil {
		return fileRec, nil
	}

	cpRec, err := store.GetRespawnCounter(beadID)
	if err != nil || cpRec == nil {
		if fileRec != nil {
			_ = store.UpsertRespawnCounter(controlplane.RespawnCounter{
				BeadID:      beadID,
				Rig:         rigForBead(townRoot, beadID),
				Count:       fileRec.Count,
				MaxCount:    config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV(),
				LastRespawn: fileRec.LastRespawn.Format(time.RFC3339),
				Blocked:     fileRec.Count >= config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV(),
				UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
				Evidence: map[string]interface{}{
					"projection_file": beadRespawnStateFile(townRoot),
					"migrated":        true,
				},
			})
		}
		return fileRec, store
	}

	merged := &beadRespawnRecord{
		BeadID:      beadID,
		Count:       cpRec.Count,
		LastRespawn: parseMaybeRFC3339(cpRec.LastRespawn),
	}
	if fileRec != nil && (fileRec.Count > merged.Count || (fileRec.Count == merged.Count && fileRec.LastRespawn.After(merged.LastRespawn))) {
		merged.Count = fileRec.Count
		merged.LastRespawn = fileRec.LastRespawn
	}

	_ = store.UpsertRespawnCounter(controlplane.RespawnCounter{
		BeadID:      beadID,
		Rig:         rigForBead(townRoot, beadID),
		Count:       merged.Count,
		MaxCount:    max(cpRec.MaxCount, config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV()),
		LastRespawn: merged.LastRespawn.Format(time.RFC3339),
		Blocked:     merged.Count >= config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV(),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		Evidence: map[string]interface{}{
			"projection_file": beadRespawnStateFile(townRoot),
			"source":          "reconciled",
		},
	})
	persistRespawnProjection(townRoot, merged)
	return merged, store
}

func persistRespawnProjection(townRoot string, rec *beadRespawnRecord) {
	state := loadBeadRespawnState(townRoot)
	if state.Beads == nil {
		state.Beads = make(map[string]*beadRespawnRecord)
	}
	copyRec := *rec
	state.Beads[rec.BeadID] = &copyRec
	_ = saveBeadRespawnState(townRoot, state)
}

func parseMaybeRFC3339(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func rigForBead(townRoot, beadID string) string {
	prefix := beads.ExtractPrefix(beadID)
	if prefix == "" {
		return ""
	}
	return beads.GetRigNameForPrefix(townRoot, prefix)
}

func witnessActor(townRoot, beadID string) string {
	if rig := rigForBead(townRoot, beadID); rig != "" {
		return fmt.Sprintf("%s/witness", rig)
	}
	return "witness"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
