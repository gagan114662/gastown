package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

type memoryStatus string

const (
	memoryStatusActive     memoryStatus = "active"
	memoryStatusArchived   memoryStatus = "archived"
	memoryStatusSuperseded memoryStatus = "superseded"
	memoryStatusDraft      memoryStatus = "draft"
)

type memoryRecord struct {
	Type            string       `json:"type,omitempty"`
	Content         string       `json:"content"`
	Scope           string       `json:"scope,omitempty"`
	Source          string       `json:"source,omitempty"`
	Confidence      float64      `json:"confidence,omitempty"`
	Status          memoryStatus `json:"status,omitempty"`
	Supersedes      []string     `json:"supersedes,omitempty"`
	LastValidatedAt string       `json:"last_validated_at,omitempty"`
	UpdatedAt       string       `json:"updated_at,omitempty"`
}

type memoryPolicySettings struct {
	MinConfidence  float64
	AllowedStatus  map[memoryStatus]bool
	StaleAfterDays int
}

func newMemoryRecord(memType, content string) memoryRecord {
	return normalizeMemoryRecord(memoryRecord{
		Type:       memType,
		Content:    strings.TrimSpace(content),
		Scope:      "town",
		Source:     "manual",
		Confidence: 1.0,
		Status:     memoryStatusActive,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

func normalizeMemoryRecord(record memoryRecord) memoryRecord {
	record.Type = strings.TrimSpace(record.Type)
	record.Content = strings.TrimSpace(record.Content)
	record.Scope = strings.TrimSpace(record.Scope)
	if record.Scope == "" {
		record.Scope = "town"
	}
	record.Source = strings.TrimSpace(record.Source)
	if record.Source == "" {
		record.Source = "manual"
	}
	if record.Confidence <= 0 {
		record.Confidence = 1.0
	}
	if record.Confidence > 1 {
		record.Confidence = 1
	}
	switch record.Status {
	case memoryStatusActive, memoryStatusArchived, memoryStatusSuperseded, memoryStatusDraft:
	default:
		record.Status = memoryStatusActive
	}
	record.Supersedes = dedupeStrings(record.Supersedes)
	if strings.TrimSpace(record.UpdatedAt) == "" {
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return record
}

func encodeMemoryRecord(record memoryRecord) (string, error) {
	record = normalizeMemoryRecord(record)
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeMemoryRecord(memType, raw string) memoryRecord {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return normalizeMemoryRecord(memoryRecord{Type: memType})
	}
	var record memoryRecord
	if json.Unmarshal([]byte(raw), &record) == nil && strings.TrimSpace(record.Content) != "" {
		if record.Type == "" {
			record.Type = memType
		}
		return normalizeMemoryRecord(record)
	}
	return normalizeMemoryRecord(memoryRecord{
		Type:            memType,
		Content:         raw,
		Scope:           "town",
		Source:          "legacy-kv",
		Confidence:      1.0,
		Status:          memoryStatusActive,
		LastValidatedAt: "",
	})
}

func memoryEligibleForPrime(record memoryRecord, policy memoryPolicySettings) bool {
	if len(policy.AllowedStatus) == 0 {
		policy = defaultMemoryPolicy()
	}
	return policy.AllowedStatus[record.Status] && record.Confidence >= policy.MinConfidence
}

func memoryMetadataLine(record memoryRecord) string {
	var parts []string
	parts = append(parts, "status="+string(record.Status))
	parts = append(parts, fmt.Sprintf("confidence=%.2f", record.Confidence))
	if record.Scope != "" {
		parts = append(parts, "scope="+record.Scope)
	}
	if record.Source != "" {
		parts = append(parts, "source="+record.Source)
	}
	if record.LastValidatedAt != "" {
		parts = append(parts, "validated="+record.LastValidatedAt)
	}
	if len(record.Supersedes) > 0 {
		parts = append(parts, "supersedes="+strings.Join(record.Supersedes, ","))
	}
	return strings.Join(parts, " · ")
}

func loadMemoryPolicy(townRoot, rigName string) memoryPolicySettings {
	policy := defaultMemoryPolicy()
	if townRoot == "" {
		return policy
	}

	if townSettings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(townRoot)); err == nil {
		policy = mergeMemoryPolicy(policy, townSettings.Memory)
	}

	if rigName != "" {
		rigPath := filepath.Join(townRoot, rigName)
		if rigSettings, err := config.LoadEffectiveRigSettings(rigPath, filepath.Join(rigPath, "mayor", "rig")); err == nil {
			policy = mergeMemoryPolicy(policy, rigSettings.Memory)
		}
	}
	return policy
}

func defaultMemoryPolicy() memoryPolicySettings {
	cfg := config.DefaultMemoryConfig()
	policy := memoryPolicySettings{
		MinConfidence:  cfg.MinConfidence,
		AllowedStatus:  map[memoryStatus]bool{},
		StaleAfterDays: cfg.StaleAfterDays,
	}
	for _, status := range cfg.AllowedStatuses {
		policy.AllowedStatus[memoryStatus(status)] = true
	}
	return policy
}

func mergeMemoryPolicy(base memoryPolicySettings, override *config.MemoryConfig) memoryPolicySettings {
	if override == nil {
		return base
	}
	if override.MinConfidence > 0 {
		base.MinConfidence = override.MinConfidence
	}
	if override.StaleAfterDays > 0 {
		base.StaleAfterDays = override.StaleAfterDays
	}
	if len(override.AllowedStatuses) > 0 {
		base.AllowedStatus = map[memoryStatus]bool{}
		for _, status := range override.AllowedStatuses {
			base.AllowedStatus[memoryStatus(strings.TrimSpace(status))] = true
		}
	}
	return base
}

func parseMemoryTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
