package ctxstack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const dbFileName = "context.sqlite"

type Store struct {
	townRoot string
	dbPath   string
}

func Open(townRoot string) (*Store, error) {
	if strings.TrimSpace(townRoot) == "" {
		return nil, fmt.Errorf("town root is required")
	}
	runtimeDir := filepath.Join(townRoot, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return nil, fmt.Errorf("creating runtime dir: %w", err)
	}
	s := &Store{
		townRoot: townRoot,
		dbPath:   filepath.Join(runtimeDir, dbFileName),
	}
	if err := s.EnsureSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Path() string {
	return s.dbPath
}

func (s *Store) EnsureSchema() error {
	schema := `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS session_summaries (
	session_id TEXT NOT NULL,
	role TEXT,
	rig TEXT,
	agent TEXT,
	work_bead TEXT,
	source TEXT,
	summary TEXT NOT NULL,
	changes TEXT,
	validation TEXT,
	blockers TEXT,
	next_steps TEXT,
	tags_json TEXT,
	created_at TEXT NOT NULL,
	PRIMARY KEY(session_id, source, created_at)
);
CREATE INDEX IF NOT EXISTS idx_session_summaries_lookup ON session_summaries(role, rig, work_bead, created_at DESC);
CREATE TABLE IF NOT EXISTS scratchpad_entries (
	session_id TEXT NOT NULL,
	seq INTEGER NOT NULL,
	kind TEXT NOT NULL,
	text TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY(session_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_scratchpad_session ON scratchpad_entries(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS entropy_samples (
	session_id TEXT NOT NULL,
	score REAL NOT NULL,
	band TEXT NOT NULL,
	reasons_json TEXT,
	context_usage REAL,
	action TEXT,
	created_at TEXT NOT NULL,
	PRIMARY KEY(session_id, created_at)
);
CREATE INDEX IF NOT EXISTS idx_entropy_session ON entropy_samples(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS retrieval_docs (
	doc_id TEXT PRIMARY KEY,
	tier TEXT NOT NULL,
	source TEXT,
	rig TEXT,
	role TEXT,
	bead TEXT,
	agent TEXT,
	session_id TEXT,
	tags_json TEXT,
	text TEXT NOT NULL,
	rank_features_json TEXT,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_retrieval_lookup ON retrieval_docs(tier, rig, role, bead, updated_at DESC);
CREATE VIRTUAL TABLE IF NOT EXISTS retrieval_docs_fts USING fts5(
	doc_id UNINDEXED,
	text,
	tags,
	tokenize='unicode61'
);
`
	return s.execSQL(schema)
}

func (s *Store) PutSessionSummary(summary SessionSummary) error {
	if summary.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if strings.TrimSpace(summary.Summary) == "" {
		return fmt.Errorf("summary text is required")
	}
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = time.Now().UTC()
	}
	sql := fmt.Sprintf(`
INSERT OR REPLACE INTO session_summaries (
	session_id, role, rig, agent, work_bead, source, summary, changes, validation, blockers, next_steps, tags_json, created_at
) VALUES (
	%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
);`,
		sqlText(summary.SessionID),
		sqlText(summary.Role),
		sqlText(summary.Rig),
		sqlText(summary.Agent),
		sqlText(summary.WorkBead),
		sqlText(summary.Source),
		sqlText(summary.Summary),
		sqlText(summary.Changes),
		sqlText(summary.Validation),
		sqlText(summary.Blockers),
		sqlText(summary.NextSteps),
		sqlJSON(summary.Tags),
		sqlText(summary.CreatedAt.UTC().Format(time.RFC3339)),
	)
	if err := s.execSQL(sql); err != nil {
		return err
	}
	return s.UpsertRetrievalDoc(summaryToDoc(summary))
}

func (s *Store) ListSessionSummaries(filter SummaryFilter) ([]SessionSummary, error) {
	where := []string{"1=1"}
	if filter.Role != "" {
		where = append(where, "role = "+sqlText(filter.Role))
	}
	if filter.Rig != "" {
		where = append(where, "rig = "+sqlText(filter.Rig))
	}
	if filter.Agent != "" {
		where = append(where, "agent = "+sqlText(filter.Agent))
	}
	if filter.WorkBead != "" {
		where = append(where, "work_bead = "+sqlText(filter.WorkBead))
	}
	if filter.Source != "" {
		where = append(where, "source = "+sqlText(filter.Source))
	}
	if filter.Session != "" {
		where = append(where, "session_id = "+sqlText(filter.Session))
	}
	if !filter.Since.IsZero() {
		where = append(where, "created_at >= "+sqlText(filter.Since.UTC().Format(time.RFC3339)))
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	sql := fmt.Sprintf(`
SELECT session_id, role, rig, agent, work_bead, source, summary, changes, validation, blockers, next_steps, tags_json, created_at
FROM session_summaries
WHERE %s
ORDER BY datetime(created_at) DESC
LIMIT %d;`, strings.Join(where, " AND "), limit)
	var rows []summaryRow
	if err := s.queryJSON(sql, &rows); err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.decode())
	}
	return out, nil
}

func (s *Store) AddScratchpadEntry(entry ScratchpadEntry) error {
	if entry.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if entry.Kind == "" {
		entry.Kind = "note"
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.Seq <= 0 {
		next, err := s.nextScratchpadSeq(entry.SessionID)
		if err != nil {
			return err
		}
		entry.Seq = next
	}
	sql := fmt.Sprintf(`
INSERT OR REPLACE INTO scratchpad_entries (session_id, seq, kind, text, created_at)
VALUES (%s, %d, %s, %s, %s);`,
		sqlText(entry.SessionID),
		entry.Seq,
		sqlText(entry.Kind),
		sqlText(entry.Text),
		sqlText(entry.CreatedAt.UTC().Format(time.RFC3339)),
	)
	return s.execSQL(sql)
}

func (s *Store) ListScratchpad(sessionID string, limit int) ([]ScratchpadEntry, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	sql := fmt.Sprintf(`
SELECT session_id, seq, kind, text, created_at
FROM scratchpad_entries
WHERE session_id = %s
ORDER BY seq ASC
LIMIT %d;`, sqlText(sessionID), limit)
	var rows []scratchpadRow
	if err := s.queryJSON(sql, &rows); err != nil {
		return nil, err
	}
	out := make([]ScratchpadEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.decode())
	}
	return out, nil
}

func (s *Store) ClearScratchpad(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	return s.execSQL("DELETE FROM scratchpad_entries WHERE session_id = " + sqlText(sessionID) + ";")
}

func (s *Store) PutEntropySample(sample EntropySample) error {
	if sample.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if sample.CreatedAt.IsZero() {
		sample.CreatedAt = time.Now().UTC()
	}
	sql := fmt.Sprintf(`
INSERT OR REPLACE INTO entropy_samples (session_id, score, band, reasons_json, context_usage, action, created_at)
VALUES (%s, %.6f, %s, %s, %s, %s, %s);`,
		sqlText(sample.SessionID),
		sample.Score,
		sqlText(sample.Band),
		sqlJSON(sample.Reasons),
		sqlFloat(sample.ContextUsage),
		sqlText(sample.Action),
		sqlText(sample.CreatedAt.UTC().Format(time.RFC3339)),
	)
	return s.execSQL(sql)
}

func (s *Store) LatestEntropySample(sessionID string) (*EntropySample, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	sql := fmt.Sprintf(`
SELECT session_id, score, band, reasons_json, context_usage, action, created_at
FROM entropy_samples
WHERE session_id = %s
ORDER BY datetime(created_at) DESC
LIMIT 1;`, sqlText(sessionID))
	var rows []entropyRow
	if err := s.queryJSON(sql, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	sample := rows[0].decode()
	return &sample, nil
}

func (s *Store) UpsertRetrievalDoc(doc RetrievalDoc) error {
	if doc.DocID == "" {
		return fmt.Errorf("doc_id is required")
	}
	if doc.Tier == "" {
		doc.Tier = TierCold
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = time.Now().UTC()
	}
	sql := fmt.Sprintf(`
INSERT OR REPLACE INTO retrieval_docs (
	doc_id, tier, source, rig, role, bead, agent, session_id, tags_json, text, rank_features_json, updated_at
) VALUES (
	%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s
);`,
		sqlText(doc.DocID),
		sqlText(doc.Tier),
		sqlText(doc.Source),
		sqlText(doc.Rig),
		sqlText(doc.Role),
		sqlText(doc.Bead),
		sqlText(doc.Agent),
		sqlText(doc.SessionID),
		sqlJSON(doc.Tags),
		sqlText(doc.Text),
		sqlJSON(doc.RankFeatures),
		sqlText(doc.UpdatedAt.UTC().Format(time.RFC3339)),
	)
	if err := s.execSQL(sql); err != nil {
		return err
	}
	return s.syncFTS()
}

func (s *Store) DeleteRetrievalDoc(docID string) error {
	if docID == "" {
		return nil
	}
	if err := s.execSQL("DELETE FROM retrieval_docs WHERE doc_id = " + sqlText(docID) + ";"); err != nil {
		return err
	}
	return s.syncFTS()
}

func (s *Store) SearchRetrieval(opts SearchOptions) ([]RetrievalDoc, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 8
	}
	maxFetch := opts.MaxFetch
	if maxFetch <= 0 {
		maxFetch = limit * 4
	}
	where := []string{"1=1"}
	if opts.Tier != "" {
		where = append(where, "d.tier = "+sqlText(opts.Tier))
	}
	if opts.Source != "" {
		where = append(where, "d.source = "+sqlText(opts.Source))
	}
	if opts.Rig != "" {
		where = append(where, "d.rig = "+sqlText(opts.Rig))
	}
	if opts.Role != "" {
		where = append(where, "d.role = "+sqlText(opts.Role))
	}
	if opts.Agent != "" {
		where = append(where, "d.agent = "+sqlText(opts.Agent))
	}
	if opts.Bead != "" {
		where = append(where, "d.bead = "+sqlText(opts.Bead))
	}
	if opts.Session != "" {
		where = append(where, "d.session_id = "+sqlText(opts.Session))
	}

	var sql string
	if query := buildFTSQuery(opts.Query); query != "" {
		sql = fmt.Sprintf(`
SELECT d.doc_id, d.tier, d.source, d.rig, d.role, d.bead, d.agent, d.session_id, d.tags_json, d.text, d.rank_features_json, d.updated_at, bm25(retrieval_docs_fts) AS bm25
FROM retrieval_docs d
JOIN retrieval_docs_fts ON retrieval_docs_fts.doc_id = d.doc_id
WHERE retrieval_docs_fts MATCH %s AND %s
ORDER BY bm25 ASC, datetime(d.updated_at) DESC
LIMIT %d;`, sqlText(query), strings.Join(where, " AND "), maxFetch)
	} else {
		sql = fmt.Sprintf(`
SELECT d.doc_id, d.tier, d.source, d.rig, d.role, d.bead, d.agent, d.session_id, d.tags_json, d.text, d.rank_features_json, d.updated_at, 0 AS bm25
FROM retrieval_docs d
WHERE %s
ORDER BY datetime(d.updated_at) DESC
LIMIT %d;`, strings.Join(where, " AND "), maxFetch)
	}
	var rows []retrievalRow
	if err := s.queryJSON(sql, &rows); err != nil {
		return nil, err
	}
	out := make([]RetrievalDoc, 0, len(rows))
	for _, row := range rows {
		doc := row.decode()
		doc.SearchScore = rankBoost(doc, opts) - row.BM25
		out = append(out, doc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SearchScore == out[j].SearchScore {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return out[i].SearchScore > out[j].SearchScore
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) BuildPrimeSnapshot(req PrimeRequest, settings Settings, caps RuntimeCapabilities) (*PrimeSnapshot, error) {
	maxItems := req.MaxItems
	if maxItems <= 0 {
		maxItems = 6
	}
	snapshot := &PrimeSnapshot{
		Budget: settings.Allocate(caps),
	}

	summaries, err := s.ListSessionSummaries(SummaryFilter{
		Role:     req.Role,
		Rig:      req.Rig,
		WorkBead: req.WorkBead,
		Limit:    4,
	})
	if err != nil {
		return nil, err
	}
	if len(summaries) > 0 {
		snapshot.PrimarySummary = &summaries[0]
		if len(summaries) > 1 {
			snapshot.Recent = summaries[1:]
		}
	}

	docs, err := s.SearchRetrieval(SearchOptions{
		Query:   req.Query,
		Rig:     req.Rig,
		Role:    req.Role,
		Agent:   req.Agent,
		Bead:    req.WorkBead,
		Session: req.SessionID,
		Limit:   maxItems,
	})
	if err != nil {
		return nil, err
	}
	snapshot.Docs = docs
	return snapshot, nil
}

func (s *Store) LatestActivityAt(sessionID string) (time.Time, error) {
	if sessionID == "" {
		return time.Time{}, nil
	}
	sql := fmt.Sprintf(`
SELECT MAX(ts) AS ts FROM (
	SELECT created_at AS ts FROM session_summaries WHERE session_id = %s
	UNION ALL
	SELECT created_at AS ts FROM scratchpad_entries WHERE session_id = %s
	UNION ALL
	SELECT created_at AS ts FROM entropy_samples WHERE session_id = %s
);`, sqlText(sessionID), sqlText(sessionID), sqlText(sessionID))
	var rows []struct {
		TS string `json:"ts"`
	}
	if err := s.queryJSON(sql, &rows); err != nil {
		return time.Time{}, err
	}
	if len(rows) == 0 || strings.TrimSpace(rows[0].TS) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, rows[0].TS)
}

func summaryToDoc(summary SessionSummary) RetrievalDoc {
	text := strings.TrimSpace(strings.Join([]string{
		summary.Summary,
		withLabel("Changes", summary.Changes),
		withLabel("Validation", summary.Validation),
		withLabel("Blockers", summary.Blockers),
		withLabel("Next steps", summary.NextSteps),
	}, "\n\n"))
	docID := "summary:" + summary.SessionID + ":" + summary.CreatedAt.UTC().Format("20060102T150405")
	tags := append([]string(nil), summary.Tags...)
	if summary.Source != "" {
		tags = append(tags, "source:"+summary.Source)
	}
	return RetrievalDoc{
		DocID:         docID,
		Tier:          TierWarm,
		Source:        "session_summary",
		Rig:           summary.Rig,
		Role:          summary.Role,
		Bead:          summary.WorkBead,
		Agent:         summary.Agent,
		SessionID:     summary.SessionID,
		Tags:          dedupe(tags),
		Text:          text,
		UpdatedAt:     summary.CreatedAt,
		SummarySource: summary.Source,
	}
}

func withLabel(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func buildFTSQuery(query string) string {
	tokens := strings.Fields(strings.ToLower(query))
	clean := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return r
			case r >= '0' && r <= '9':
				return r
			default:
				return -1
			}
		}, token)
		if token != "" {
			clean = append(clean, token+"*")
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return strings.Join(clean, " OR ")
}

func rankBoost(doc RetrievalDoc, opts SearchOptions) float64 {
	score := 1.0
	if opts.Rig != "" && doc.Rig == opts.Rig {
		score += 2.5
	}
	if opts.Role != "" && doc.Role == opts.Role {
		score += 2.5
	}
	if opts.Bead != "" && doc.Bead == opts.Bead {
		score += 4
	}
	if opts.Agent != "" && doc.Agent == opts.Agent {
		score += 1
	}
	if doc.Tier == TierWarm {
		score += 1.5
	}
	if doc.Source == "session_summary" {
		score += 1.5
	}
	return score
}

func (s *Store) nextScratchpadSeq(sessionID string) (int, error) {
	sql := fmt.Sprintf("SELECT COALESCE(MAX(seq), 0) + 1 AS next_seq FROM scratchpad_entries WHERE session_id = %s;", sqlText(sessionID))
	var rows []struct {
		NextSeq int `json:"next_seq"`
	}
	if err := s.queryJSON(sql, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 || rows[0].NextSeq <= 0 {
		return 1, nil
	}
	return rows[0].NextSeq, nil
}

func (s *Store) execSQL(sql string) error {
	cmd := exec.Command("sqlite3", "-cmd", ".timeout 5000", s.dbPath, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("sqlite exec: %s", msg)
	}
	return nil
}

func (s *Store) syncFTS() error {
	sql := `
DELETE FROM retrieval_docs_fts;
INSERT INTO retrieval_docs_fts(doc_id, text, tags)
SELECT doc_id, text, COALESCE(tags_json, '')
FROM retrieval_docs;
`
	return s.execSQL(sql)
}

func (s *Store) queryJSON(sql string, dest any) error {
	cmd := exec.Command("sqlite3", "-json", "-cmd", ".timeout 5000", s.dbPath, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("sqlite query: %s", msg)
	}
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		trimmed = []byte("[]")
	}
	if err := json.Unmarshal(trimmed, dest); err != nil {
		return fmt.Errorf("parsing sqlite json: %w", err)
	}
	return nil
}

func sqlText(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func sqlFloat(v float64) string {
	if v == 0 {
		return "NULL"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func sqlJSON(v any) string {
	if v == nil {
		return "NULL"
	}
	data, err := json.Marshal(v)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return "NULL"
	}
	return sqlText(string(data))
}

func dedupe(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type summaryRow struct {
	SessionID  string `json:"session_id"`
	Role       string `json:"role"`
	Rig        string `json:"rig"`
	Agent      string `json:"agent"`
	WorkBead   string `json:"work_bead"`
	Source     string `json:"source"`
	Summary    string `json:"summary"`
	Changes    string `json:"changes"`
	Validation string `json:"validation"`
	Blockers   string `json:"blockers"`
	NextSteps  string `json:"next_steps"`
	TagsJSON   string `json:"tags_json"`
	CreatedAt  string `json:"created_at"`
}

func (r summaryRow) decode() SessionSummary {
	return SessionSummary{
		SessionID:  r.SessionID,
		Role:       r.Role,
		Rig:        r.Rig,
		Agent:      r.Agent,
		WorkBead:   r.WorkBead,
		Source:     r.Source,
		Summary:    r.Summary,
		Changes:    r.Changes,
		Validation: r.Validation,
		Blockers:   r.Blockers,
		NextSteps:  r.NextSteps,
		Tags:       parseStringSlice(r.TagsJSON),
		CreatedAt:  parseTime(r.CreatedAt),
	}
}

type scratchpadRow struct {
	SessionID string `json:"session_id"`
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

func (r scratchpadRow) decode() ScratchpadEntry {
	return ScratchpadEntry{
		SessionID: r.SessionID,
		Seq:       r.Seq,
		Kind:      r.Kind,
		Text:      r.Text,
		CreatedAt: parseTime(r.CreatedAt),
	}
}

type entropyRow struct {
	SessionID    string  `json:"session_id"`
	Score        float64 `json:"score"`
	Band         string  `json:"band"`
	ReasonsJSON  string  `json:"reasons_json"`
	ContextUsage float64 `json:"context_usage"`
	Action       string  `json:"action"`
	CreatedAt    string  `json:"created_at"`
}

func (r entropyRow) decode() EntropySample {
	return EntropySample{
		SessionID:    r.SessionID,
		Score:        r.Score,
		Band:         r.Band,
		Reasons:      parseStringSlice(r.ReasonsJSON),
		ContextUsage: r.ContextUsage,
		Action:       r.Action,
		CreatedAt:    parseTime(r.CreatedAt),
	}
}

type retrievalRow struct {
	DocID            string  `json:"doc_id"`
	Tier             string  `json:"tier"`
	Source           string  `json:"source"`
	Rig              string  `json:"rig"`
	Role             string  `json:"role"`
	Bead             string  `json:"bead"`
	Agent            string  `json:"agent"`
	SessionID        string  `json:"session_id"`
	TagsJSON         string  `json:"tags_json"`
	Text             string  `json:"text"`
	RankFeaturesJSON string  `json:"rank_features_json"`
	UpdatedAt        string  `json:"updated_at"`
	BM25             float64 `json:"bm25"`
}

func (r retrievalRow) decode() RetrievalDoc {
	return RetrievalDoc{
		DocID:        r.DocID,
		Tier:         r.Tier,
		Source:       r.Source,
		Rig:          r.Rig,
		Role:         r.Role,
		Bead:         r.Bead,
		Agent:        r.Agent,
		SessionID:    r.SessionID,
		Tags:         parseStringSlice(r.TagsJSON),
		Text:         r.Text,
		RankFeatures: parseObject(r.RankFeaturesJSON),
		UpdatedAt:    parseTime(r.UpdatedAt),
	}
}

func parseStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func parseObject(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func parseTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}
