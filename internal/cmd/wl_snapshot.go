package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/wasteland"
)

func encodeJSONSQL(value any) string {
	if value == nil {
		return "NULL"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", doltserver.EscapeSQL(string(data)))
}

func decodeWantedWorkSpecField(raw string) *doltserver.WantedWorkSpec {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return nil
	}
	var spec doltserver.WantedWorkSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil
	}
	if spec.Version == 0 {
		spec.Version = 1
	}
	return &spec
}

func decodeCompletionSnapshotField(raw string) *doltserver.CompletionStatusSnapshot {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return nil
	}
	var snapshot doltserver.CompletionStatusSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil
	}
	if snapshot.Version == 0 {
		snapshot.Version = 1
	}
	return &snapshot
}

func buildCompletionSnapshot(localDir, rigHandle, claimStatus string, status wasteland.ValidationStatus, submittedAt string, existing *doltserver.CompletionStatusSnapshot) *doltserver.CompletionStatusSnapshot {
	snapshot := &doltserver.CompletionStatusSnapshot{
		Version:          1,
		RigHandle:        strings.TrimSpace(rigHandle),
		ClaimStatus:      strings.TrimSpace(claimStatus),
		ValidationStatus: string(status),
		SubmittedAt:      strings.TrimSpace(submittedAt),
	}

	if existing != nil {
		if snapshot.RigHandle == "" {
			snapshot.RigHandle = existing.RigHandle
		}
		if snapshot.ClaimStatus == "" {
			snapshot.ClaimStatus = existing.ClaimStatus
		}
		if snapshot.SubmittedAt == "" {
			snapshot.SubmittedAt = existing.SubmittedAt
		}
		if existing.TrustTier != "" {
			snapshot.TrustTier = existing.TrustTier
		}
	}

	if snapshot.ClaimStatus == "" {
		snapshot.ClaimStatus = "in_review"
	}
	if snapshot.SubmittedAt == "" {
		snapshot.SubmittedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if level, err := queryRigTrustLevelInLocalClone(localDir, snapshot.RigHandle); err == nil {
		snapshot.TrustTier = wasteland.TrustTier(level).String()
	}
	if snapshot.TrustTier == "" {
		snapshot.TrustTier = wasteland.TierDrifter.String()
	}
	return snapshot
}

func formatWantedWorkSpec(spec *doltserver.WantedWorkSpec) string {
	if spec == nil {
		return ""
	}
	var parts []string
	if spec.TargetRepo != "" {
		parts = append(parts, "repo: "+spec.TargetRepo)
	}
	if spec.Deliverable != "" {
		parts = append(parts, "deliverable: "+spec.Deliverable)
	}
	if spec.TargetBranch != "" {
		parts = append(parts, "branch: "+spec.TargetBranch)
	}
	if len(spec.AcceptanceNotes) > 0 {
		parts = append(parts, "acceptance: "+strings.Join(spec.AcceptanceNotes, "; "))
	}
	return strings.Join(parts, "\n")
}
