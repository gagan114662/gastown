package cmd

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/wasteland"
)

func requireWastelandTier(townRoot string, cfg *wasteland.Config, min wasteland.TrustTier, action string) error {
	level, err := currentWastelandTier(townRoot, cfg)
	if err != nil {
		return fmt.Errorf("loading trust level: %w", err)
	}
	if level < min {
		return fmt.Errorf("%s requires %s trust or higher (current: %s)", action, min.String(), level.String())
	}
	return nil
}

func currentWastelandTier(townRoot string, cfg *wasteland.Config) (wasteland.TrustTier, error) {
	if cfg == nil {
		return wasteland.TierDrifter, fmt.Errorf("missing wasteland config")
	}
	if doltserver.DatabaseExists(townRoot, doltserver.WLCommonsDB) {
		level, err := doltserver.QueryRigTrustLevel(townRoot, cfg.RigHandle)
		return wasteland.TrustTier(level), err
	}
	if cfg.LocalDir == "" {
		return wasteland.TierDrifter, fmt.Errorf("no local wasteland clone configured")
	}
	level, err := queryRigTrustLevelInLocalClone(cfg.LocalDir, cfg.RigHandle)
	return wasteland.TrustTier(level), err
}

func queryRigTrustLevelInLocalClone(localDir, handle string) (int, error) {
	rows, err := runDoltCSV(localDir, fmt.Sprintf("SELECT trust_level FROM rigs WHERE handle='%s';", doltserver.EscapeSQL(handle)))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, fmt.Errorf("rig %q not found", handle)
	}
	return strconv.Atoi(rows[0][0])
}

func queryCompletionInLocalClone(localDir, completionID string) (*doltserver.CompletionRecord, error) {
	rows, err := runDoltCSV(localDir, fmt.Sprintf("SELECT id, wanted_id, completed_by, evidence, COALESCE(evidence_type, ''), COALESCE(validation_status, ''), COALESCE(verified_by, ''), COALESCE(CAST(verified_at AS CHAR), ''), COALESCE(CAST(completed_at AS CHAR), ''), COALESCE(status_snapshot, JSON_OBJECT()) FROM completions WHERE id='%s';", doltserver.EscapeSQL(completionID)))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("completion %q not found", completionID)
	}
	row := rows[0]
	return &doltserver.CompletionRecord{
		ID:               row[0],
		WantedID:         row[1],
		CompletedBy:      row[2],
		Evidence:         row[3],
		EvidenceType:     row[4],
		ValidationStatus: row[5],
		VerifiedBy:       row[6],
		VerifiedAt:       row[7],
		CompletedAt:      row[8],
		StatusSnapshot:   decodeCompletionSnapshotField(row[9]),
	}, nil
}

func updateCompletionValidationInLocalClone(localDir, completionID, verifier string, assessment wasteland.EvidenceAssessment) error {
	completion, err := queryCompletionInLocalClone(localDir, completionID)
	if err != nil {
		return err
	}
	verifiedBy := "NULL"
	verifiedAt := "NULL"
	if verifier != "" {
		verifiedBy = fmt.Sprintf("'%s'", doltserver.EscapeSQL(verifier))
		verifiedAt = "NOW()"
	}
	snapshotField := encodeJSONSQL(buildCompletionSnapshot(localDir, completion.CompletedBy, "in_review", assessment.Status, completion.CompletedAt, completion.StatusSnapshot))
	script := fmt.Sprintf(`UPDATE completions SET evidence_type='%s', validation_status='%s', verified_by=%s, verified_at=%s, status_snapshot=%s WHERE id='%s';
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'wl verify: %s');`,
		doltserver.EscapeSQL(string(assessment.Type)),
		doltserver.EscapeSQL(string(assessment.Status)),
		verifiedBy,
		verifiedAt,
		snapshotField,
		doltserver.EscapeSQL(completionID),
		doltserver.EscapeSQL(completionID),
	)
	cmd := exec.Command("dolt", "sql", "-q", script)
	cmd.Dir = localDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating completion validation: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runDoltCSV(localDir, query string) ([][]string, error) {
	cmd := exec.Command("dolt", "sql", "-r", "csv", "-q", query)
	cmd.Dir = localDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) <= 1 {
		return nil, nil
	}
	return records[1:], nil
}
