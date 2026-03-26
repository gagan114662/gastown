// Package deacon provides the Deacon agent infrastructure.
package deacon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/controlplane"
	"github.com/steveyegge/gastown/internal/events"
)

// Default parameters for re-dispatch rate-limiting.
// Configurable via operational.deacon.max_redispatches and
// operational.deacon.redispatch_cooldown in settings/config.json.
const (
	// DefaultMaxRedispatches is the number of times a bead can be re-dispatched
	// before escalating to Mayor instead of re-slinging.
	DefaultMaxRedispatches = 3

	// DefaultRedispatchCooldown is the minimum time between re-dispatches of
	// the same bead. Prevents thrashing when a bead keeps killing polecats.
	DefaultRedispatchCooldown = 5 * time.Minute
)

// RedispatchState tracks re-dispatch attempts for recovered beads.
// Persisted to deacon/redispatch-state.json.
type RedispatchState struct {
	// Beads maps bead ID to their re-dispatch tracking state.
	Beads map[string]*BeadRedispatchState `json:"beads"`

	// LastUpdated is when this state was last written.
	LastUpdated time.Time `json:"last_updated"`
}

// BeadRedispatchState tracks the re-dispatch history for a single bead.
type BeadRedispatchState struct {
	// BeadID is the bead identifier.
	BeadID string `json:"bead_id"`

	// AttemptCount is total number of re-dispatch attempts for this bead.
	AttemptCount int `json:"attempt_count"`

	// LastAttemptTime is when the last re-dispatch was attempted.
	LastAttemptTime time.Time `json:"last_attempt_time,omitempty"`

	// LastRig is the rig where the last re-dispatch was sent.
	LastRig string `json:"last_rig,omitempty"`

	// Escalated is true if this bead has been escalated to Mayor.
	Escalated bool `json:"escalated,omitempty"`

	// EscalatedAt is when the bead was escalated.
	EscalatedAt time.Time `json:"escalated_at,omitempty"`
}

// RedispatchResult describes the outcome of a re-dispatch attempt.
type RedispatchResult struct {
	BeadID    string `json:"bead_id"`
	Action    string `json:"action"` // "redispatched", "cooldown", "escalated", "error"
	TargetRig string `json:"target_rig,omitempty"`
	Attempts  int    `json:"attempts"`
	Message   string `json:"message,omitempty"`
	Error     error  `json:"error,omitempty"`
}

// RedispatchStateFile returns the path to the re-dispatch state file.
func RedispatchStateFile(townRoot string) string {
	return filepath.Join(townRoot, "deacon", "redispatch-state.json")
}

// LoadRedispatchState loads the re-dispatch state from disk.
// Returns empty state if file doesn't exist.
func LoadRedispatchState(townRoot string) (*RedispatchState, error) {
	stateFile := RedispatchStateFile(townRoot)

	data, err := os.ReadFile(stateFile) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return &RedispatchState{
				Beads: make(map[string]*BeadRedispatchState),
			}, nil
		}
		return nil, fmt.Errorf("reading redispatch state: %w", err)
	}

	var state RedispatchState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing redispatch state: %w", err)
	}

	if state.Beads == nil {
		state.Beads = make(map[string]*BeadRedispatchState)
	}

	return &state, nil
}

// SaveRedispatchState saves the re-dispatch state to disk.
func SaveRedispatchState(townRoot string, state *RedispatchState) error {
	stateFile := RedispatchStateFile(townRoot)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating deacon directory: %w", err)
	}

	state.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling redispatch state: %w", err)
	}

	return os.WriteFile(stateFile, data, 0600)
}

// GetBeadState returns the re-dispatch state for a bead, creating if needed.
func (s *RedispatchState) GetBeadState(beadID string) *BeadRedispatchState {
	if s.Beads == nil {
		s.Beads = make(map[string]*BeadRedispatchState)
	}

	state, ok := s.Beads[beadID]
	if !ok {
		state = &BeadRedispatchState{BeadID: beadID}
		s.Beads[beadID] = state
	}
	return state
}

// IsInCooldown returns true if the bead was recently re-dispatched.
func (s *BeadRedispatchState) IsInCooldown(cooldown time.Duration) bool {
	if s.LastAttemptTime.IsZero() {
		return false
	}
	return time.Since(s.LastAttemptTime) < cooldown
}

// CooldownRemaining returns how long until cooldown expires.
func (s *BeadRedispatchState) CooldownRemaining(cooldown time.Duration) time.Duration {
	if s.LastAttemptTime.IsZero() {
		return 0
	}
	remaining := cooldown - time.Since(s.LastAttemptTime)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ShouldEscalate returns true if the bead has exceeded the max re-dispatch attempts.
func (s *BeadRedispatchState) ShouldEscalate(maxAttempts int) bool {
	return s.AttemptCount >= maxAttempts
}

// RecordAttempt records a re-dispatch attempt for the bead.
func (s *BeadRedispatchState) RecordAttempt(rig string) {
	s.AttemptCount++
	s.LastAttemptTime = time.Now().UTC()
	s.LastRig = rig
}

// RecordEscalation records that the bead was escalated to Mayor.
func (s *BeadRedispatchState) RecordEscalation() {
	s.Escalated = true
	s.EscalatedAt = time.Now().UTC()
}

// Redispatch handles a RECOVERED_BEAD message by re-slinging the bead to an
// available polecat, or escalating to Mayor if the bead has failed too many times.
//
// Parameters:
//   - townRoot: the Gas Town workspace root
//   - beadID: the recovered bead to re-dispatch
//   - sourceRig: the rig from which the bead was recovered (empty = auto-detect from prefix)
//   - maxAttempts: max re-dispatches before escalating (0 = use default)
//   - cooldown: min time between re-dispatches (0 = use default)
func Redispatch(townRoot, beadID, sourceRig string, maxAttempts int, cooldown time.Duration) *RedispatchResult {
	result := &RedispatchResult{BeadID: beadID}

	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxRedispatches
	}
	if cooldown <= 0 {
		cooldown = DefaultRedispatchCooldown
	}

	// Load state
	state, err := LoadRedispatchState(townRoot)
	if err != nil {
		result.Action = "error"
		result.Error = fmt.Errorf("loading redispatch state: %w", err)
		logRedispatchDecision(townRoot, sourceRig, "", result, maxAttempts)
		return result
	}

	beadState, store := loadMergedRedispatchState(townRoot, state, beadID)
	result.Attempts = beadState.AttemptCount

	// Check if already escalated
	if beadState.Escalated {
		result.Action = "already-escalated"
		result.Message = fmt.Sprintf("bead already escalated to Mayor at %s", beadState.EscalatedAt.Format(time.RFC3339))
		persistRedispatchState(townRoot, state, beadID, sourceRig, beadState.LastRig, "already-escalated", cooldown, beadState, store)
		logRedispatchDecision(townRoot, sourceRig, beadState.LastRig, result, maxAttempts)
		return result
	}

	// Check cooldown
	if beadState.IsInCooldown(cooldown) {
		remaining := beadState.CooldownRemaining(cooldown)
		result.Action = "cooldown"
		result.Message = fmt.Sprintf("in cooldown (remaining: %s)", remaining.Round(time.Second))
		persistRedispatchState(townRoot, state, beadID, sourceRig, beadState.LastRig, "cooldown", cooldown, beadState, store)
		logRedispatchDecision(townRoot, sourceRig, beadState.LastRig, result, maxAttempts)
		return result
	}

	// Check if we should escalate instead of re-dispatching
	if beadState.ShouldEscalate(maxAttempts) {
		result.Action = "escalated"
		result.Attempts = beadState.AttemptCount

		// Escalate to Mayor
		err := escalateToMayor(townRoot, beadID, beadState)
		if err != nil {
			result.Error = fmt.Errorf("escalating to mayor: %w", err)
			result.Message = fmt.Sprintf("failed to escalate after %d attempts: %v", beadState.AttemptCount, err)
		} else {
			beadState.RecordEscalation()
			result.Message = fmt.Sprintf("escalated to Mayor after %d failed re-dispatches", beadState.AttemptCount)
		}

		persistRedispatchState(townRoot, state, beadID, sourceRig, beadState.LastRig, "escalated", cooldown, beadState, store)
		if saveErr := SaveRedispatchState(townRoot, state); saveErr != nil {
			// Log but don't fail - escalation mail was already sent
			result.Message += fmt.Sprintf(" (warning: state save failed: %v)", saveErr)
		}
		logRedispatchDecision(townRoot, sourceRig, beadState.LastRig, result, maxAttempts)

		return result
	}

	// Determine target rig
	targetRig := sourceRig
	if targetRig == "" {
		targetRig = resolveRigFromBead(townRoot, beadID)
	}
	if targetRig == "" {
		result.Action = "error"
		result.Error = fmt.Errorf("cannot determine target rig for bead %s", beadID)
		persistRedispatchState(townRoot, state, beadID, sourceRig, targetRig, "error", cooldown, beadState, store)
		logRedispatchDecision(townRoot, sourceRig, targetRig, result, maxAttempts)
		return result
	}
	result.TargetRig = targetRig

	// Verify bead is still open (not already claimed or closed).
	// Only proceed when status is explicitly "open". Empty status (query
	// failure) is treated as "not open" to avoid re-dispatching closed
	// beads when bd show fails. (gt-sy8)
	beadStatus := getBeadStatusForRedispatch(townRoot, beadID)
	if beadStatus != "open" {
		result.Action = "skipped"
		if beadStatus == "" {
			result.Message = "could not determine bead status (treating as not open)"
		} else {
			result.Message = fmt.Sprintf("bead status is %q (expected open)", beadStatus)
		}
		persistRedispatchState(townRoot, state, beadID, sourceRig, targetRig, "skipped", cooldown, beadState, store)
		logRedispatchDecision(townRoot, sourceRig, targetRig, result, maxAttempts)
		return result
	}

	// Re-dispatch via gt sling
	err = slingBead(townRoot, beadID, targetRig)
	if err != nil {
		result.Action = "error"
		result.Error = fmt.Errorf("slinging bead to %s: %w", targetRig, err)

		// Record the failed attempt
		beadState.RecordAttempt(targetRig)
		persistRedispatchState(townRoot, state, beadID, sourceRig, targetRig, "error", cooldown, beadState, store)
		logRedispatchDecision(townRoot, sourceRig, targetRig, result, maxAttempts)

		return result
	}

	// Record successful dispatch
	beadState.RecordAttempt(targetRig)
	result.Action = "redispatched"
	result.Attempts = beadState.AttemptCount
	result.Message = fmt.Sprintf("re-dispatched to %s (attempt %d/%d)", targetRig, beadState.AttemptCount, maxAttempts)

	persistRedispatchState(townRoot, state, beadID, sourceRig, targetRig, "redispatched", cooldown, beadState, store)
	if saveErr := SaveRedispatchState(townRoot, state); saveErr != nil {
		result.Message += fmt.Sprintf(" (warning: state save failed: %v)", saveErr)
	}
	logRedispatchDecision(townRoot, sourceRig, targetRig, result, maxAttempts)

	return result
}

// PruneRedispatchState removes entries for beads that are no longer open.
// Call periodically to prevent unbounded state growth.
func PruneRedispatchState(townRoot string) (int, error) {
	state, err := LoadRedispatchState(townRoot)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for beadID := range state.Beads {
		status := getBeadStatusForRedispatch(townRoot, beadID)
		// Remove entries for beads that are closed, or that we can't find
		if status == "closed" || status == "" {
			delete(state.Beads, beadID)
			if store, err := controlplane.Open(townRoot); err == nil {
				_ = store.DeleteRedispatchRecord(beadID)
			}
			pruned++
		}
	}

	if pruned > 0 {
		if err := SaveRedispatchState(townRoot, state); err != nil {
			return pruned, err
		}
	}

	return pruned, nil
}

func loadMergedRedispatchState(townRoot string, state *RedispatchState, beadID string) (*BeadRedispatchState, *controlplane.Store) {
	beadState := state.GetBeadState(beadID)
	store, err := controlplane.Open(townRoot)
	if err != nil {
		return beadState, nil
	}

	record, err := store.GetRedispatchRecord(beadID)
	if err != nil || record == nil {
		if beadState.AttemptCount > 0 || beadState.Escalated {
			persistRedispatchState(townRoot, state, beadID, "", beadState.LastRig, "projected", DefaultRedispatchCooldown, beadState, store)
		}
		return beadState, store
	}

	if record.AttemptCount > beadState.AttemptCount {
		beadState.AttemptCount = record.AttemptCount
	}
	if ts := parseRedispatchTime(record.LastAttemptTime); ts.After(beadState.LastAttemptTime) {
		beadState.LastAttemptTime = ts
	}
	if record.TargetRig != "" {
		beadState.LastRig = record.TargetRig
	} else if beadState.LastRig == "" {
		beadState.LastRig = record.SourceRig
	}
	if record.Escalated {
		beadState.Escalated = true
	}
	if ts := parseRedispatchTime(record.EscalatedAt); ts.After(beadState.EscalatedAt) {
		beadState.EscalatedAt = ts
	}
	persistRedispatchState(townRoot, state, beadID, record.SourceRig, record.TargetRig, record.LastAction, DefaultRedispatchCooldown, beadState, store)
	return beadState, store
}

func persistRedispatchState(townRoot string, state *RedispatchState, beadID, sourceRig, targetRig, action string, cooldown time.Duration, beadState *BeadRedispatchState, store *controlplane.Store) {
	_ = SaveRedispatchState(townRoot, state)
	if store == nil {
		return
	}
	cooldownUntil := ""
	if beadState != nil && !beadState.LastAttemptTime.IsZero() && cooldown > 0 {
		cooldownUntil = beadState.LastAttemptTime.Add(cooldown).UTC().Format(time.RFC3339)
	}
	lastAttempt := ""
	escalatedAt := ""
	if beadState != nil {
		if !beadState.LastAttemptTime.IsZero() {
			lastAttempt = beadState.LastAttemptTime.UTC().Format(time.RFC3339)
		}
		if !beadState.EscalatedAt.IsZero() {
			escalatedAt = beadState.EscalatedAt.UTC().Format(time.RFC3339)
		}
	}
	_ = store.UpsertRedispatchRecord(controlplane.RedispatchRecord{
		BeadID:          beadID,
		SourceRig:       sourceRig,
		TargetRig:       targetRig,
		AttemptCount:    beadState.AttemptCount,
		LastAttemptTime: lastAttempt,
		CooldownUntil:   cooldownUntil,
		Escalated:       beadState.Escalated,
		EscalatedAt:     escalatedAt,
		LastAction:      action,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		Evidence: map[string]interface{}{
			"projection_file": RedispatchStateFile(townRoot),
			"source":          "deacon",
		},
	})
}

func logRedispatchDecision(townRoot, sourceRig, targetRig string, result *RedispatchResult, maxAttempts int) {
	if result == nil {
		return
	}
	outcome := "success"
	if result.Action == "cooldown" || result.Action == "skipped" || result.Action == "already-escalated" {
		outcome = "deferred"
	}
	if result.Error != nil || result.Action == "error" {
		outcome = "error"
	}
	reason := result.Message
	if result.Error != nil {
		reason = result.Error.Error()
	}
	_ = events.LogEventAt(townRoot, events.Event{
		Kind:       events.TypeRedispatch,
		Type:       events.TypeRedispatch,
		Actor:      deaconActor(sourceRig),
		Role:       "deacon",
		Rig:        firstNonEmpty(targetRig, sourceRig),
		BeadID:     result.BeadID,
		Outcome:    outcome,
		Reason:     reason,
		Visibility: events.VisibilityAudit,
		Payload: map[string]interface{}{
			"action":       result.Action,
			"attempts":     result.Attempts,
			"max_attempts": maxAttempts,
			"source_rig":   sourceRig,
			"target_rig":   targetRig,
		},
	})
}

func parseRedispatchTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func deaconActor(rig string) string {
	if rig != "" {
		return fmt.Sprintf("%s/deacon", rig)
	}
	return "deacon"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// resolveRigFromBead determines the rig that owns a bead based on its prefix.
func resolveRigFromBead(townRoot, beadID string) string {
	prefix := beads.ExtractPrefix(beadID)
	if prefix == "" {
		return ""
	}
	return beads.GetRigNameForPrefix(townRoot, prefix)
}

// getBeadStatusForRedispatch returns the current status of a bead.
func getBeadStatusForRedispatch(townRoot, beadID string) string {
	cmd := exec.Command("bd", "show", beadID, "--json")
	cmd.Dir = townRoot

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var issues []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &issues); err != nil || len(issues) == 0 {
		return ""
	}
	return issues[0].Status
}

// slingBead dispatches a bead to a rig via gt sling.
func slingBead(townRoot, beadID, rig string) error {
	cmd := exec.Command("gt", "sling", beadID, rig, "--force", "--no-convoy")
	cmd.Dir = townRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// escalateToMayor sends an escalation mail to the Mayor about a repeatedly-failing bead.
func escalateToMayor(townRoot, beadID string, beadState *BeadRedispatchState) error {
	subject := fmt.Sprintf("REDISPATCH_FAILED: %s (%d attempts)", beadID, beadState.AttemptCount)
	body := fmt.Sprintf(`Bead %s has been recovered and re-dispatched %d times but keeps failing.

Bead: %s
Attempts: %d
Last Rig: %s
Last Attempt: %s

This bead may have a systemic issue (e.g., causes polecat crashes).
Please investigate and either:
1. Fix the underlying issue and re-sling manually
2. Close/deprioritize the bead if it's not actionable
3. Increase the re-dispatch limit if the failures were transient`,
		beadID,
		beadState.AttemptCount,
		beadID,
		beadState.AttemptCount,
		beadState.LastRig,
		beadState.LastAttemptTime.Format(time.RFC3339),
	)

	cmd := exec.Command("gt", "mail", "send", "mayor/", "-s", subject, "-m", body)
	cmd.Dir = townRoot
	return cmd.Run()
}

// ParseRecoveredBeadSubject extracts the bead ID from a RECOVERED_BEAD mail subject.
// Expected format: "RECOVERED_BEAD <bead-id>"
func ParseRecoveredBeadSubject(subject string) (beadID string, ok bool) {
	const prefix = "RECOVERED_BEAD "
	if !strings.HasPrefix(subject, prefix) {
		return "", false
	}
	beadID = strings.TrimSpace(strings.TrimPrefix(subject, prefix))
	if beadID == "" {
		return "", false
	}
	return beadID, true
}

// ParseRecoveredBeadBody extracts the source rig from a RECOVERED_BEAD mail body.
// Looks for "Polecat: <rig>/<name>" line.
func ParseRecoveredBeadBody(body string) (rig string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Polecat:") {
			polecatAddr := strings.TrimSpace(strings.TrimPrefix(line, "Polecat:"))
			parts := strings.SplitN(polecatAddr, "/", 2)
			if len(parts) >= 1 {
				return parts[0]
			}
		}
	}
	return ""
}
