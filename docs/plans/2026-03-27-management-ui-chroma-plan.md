# Management UI + Chroma Agent Memory Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a Vite+React management dashboard at `/manage` and Chroma-powered semantic agent memory to Gastown.

**Architecture:** A new `ui/` Vite+React app is embedded in the Go binary and served at `/manage`. A new `internal/chromadb/` Go package runs a local Chroma server as a daemon child process and provides embed/query helpers. Management REST endpoints live at `/api/manage/`. SSE streams live agent transcripts via `fsnotify` watching `~/.claude/projects/`.

**Tech Stack:** Go (existing), Vite + React + TypeScript, Chroma (Python server, HTTP API), fsnotify (already in go.mod), shadcn/ui + Tailwind

---

## Phase 1: Chroma Go Client

### Task 1: Chroma HTTP client

**Files:**
- Create: `internal/chromadb/client.go`
- Create: `internal/chromadb/client_test.go`

**Step 1: Write the failing test**

```go
// internal/chromadb/client_test.go
package chromadb_test

import (
    "testing"
    "github.com/steveyegge/gastown/internal/chromadb"
)

func TestNewClient(t *testing.T) {
    c := chromadb.NewClient("http://localhost:8000")
    if c == nil {
        t.Fatal("expected non-nil client")
    }
}

func TestClientPing(t *testing.T) {
    // This test requires a running Chroma server; skip if not available.
    c := chromadb.NewClient("http://localhost:8000")
    err := c.Ping()
    if err != nil {
        t.Skipf("chroma not running: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cd /path/to/gastown_2
go test ./internal/chromadb/... -v
```
Expected: FAIL with "no Go files"

**Step 3: Write minimal implementation**

```go
// internal/chromadb/client.go
package chromadb

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
    "bytes"
    "io"
)

// Client talks to a local Chroma HTTP server.
type Client struct {
    baseURL    string
    httpClient *http.Client
}

// NewClient creates a new Chroma client for the given base URL.
func NewClient(baseURL string) *Client {
    return &Client{
        baseURL: strings.TrimRight(baseURL, "/"),
        httpClient: &http.Client{Timeout: 10 * time.Second},
    }
}

// Ping checks if the Chroma server is reachable.
func (c *Client) Ping() error {
    resp, err := c.httpClient.Get(c.baseURL + "/api/v2/heartbeat")
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("chroma ping: status %d", resp.StatusCode)
    }
    return nil
}

// Document is a single item to embed.
type Document struct {
    ID       string            `json:"id"`
    Content  string            `json:"document"`
    Metadata map[string]string `json:"metadata,omitempty"`
}

// QueryResult is a single search result from Chroma.
type QueryResult struct {
    ID       string
    Content  string
    Metadata map[string]string
    Distance float64
}

// EnsureCollection creates a collection if it doesn't exist.
func (c *Client) EnsureCollection(name string) error {
    body := map[string]any{"name": name, "get_or_create": true}
    data, _ := json.Marshal(body)
    resp, err := c.httpClient.Post(c.baseURL+"/api/v2/collections", "application/json", bytes.NewReader(data))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("ensure collection %q: status %d: %s", name, resp.StatusCode, b)
    }
    return nil
}

// Upsert adds or updates documents in a collection.
func (c *Client) Upsert(collection string, docs []Document) error {
    ids := make([]string, len(docs))
    documents := make([]string, len(docs))
    metadatas := make([]map[string]string, len(docs))
    for i, d := range docs {
        ids[i] = d.ID
        documents[i] = d.Content
        metadatas[i] = d.Metadata
    }
    body := map[string]any{
        "ids":       ids,
        "documents": documents,
        "metadatas": metadatas,
    }
    data, _ := json.Marshal(body)
    resp, err := c.httpClient.Post(
        c.baseURL+"/api/v2/collections/"+collection+"/upsert",
        "application/json",
        bytes.NewReader(data),
    )
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("upsert %q: status %d: %s", collection, resp.StatusCode, b)
    }
    return nil
}

// Query searches a collection for the top-k most similar documents.
func (c *Client) Query(collection, text string, topK int) ([]QueryResult, error) {
    body := map[string]any{
        "query_texts": []string{text},
        "n_results":   topK,
        "include":     []string{"documents", "metadatas", "distances"},
    }
    data, _ := json.Marshal(body)
    resp, err := c.httpClient.Post(
        c.baseURL+"/api/v2/collections/"+collection+"/query",
        "application/json",
        bytes.NewReader(data),
    )
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("query %q: status %d: %s", collection, resp.StatusCode, b)
    }
    var result struct {
        IDs       [][]string              `json:"ids"`
        Documents [][]string              `json:"documents"`
        Metadatas [][]map[string]string   `json:"metadatas"`
        Distances [][]float64             `json:"distances"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    if len(result.IDs) == 0 {
        return nil, nil
    }
    out := make([]QueryResult, len(result.IDs[0]))
    for i := range result.IDs[0] {
        out[i] = QueryResult{
            ID:       result.IDs[0][i],
            Content:  result.Documents[0][i],
            Metadata: result.Metadatas[0][i],
            Distance: result.Distances[0][i],
        }
    }
    return out, nil
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/chromadb/... -v
```
Expected: PASS (TestClientPing is skipped if Chroma isn't running)

**Step 5: Commit**

```bash
git add internal/chromadb/client.go internal/chromadb/client_test.go
git commit -m "feat(chromadb): add Chroma HTTP client with upsert and query"
```

---

### Task 2: Chroma server lifecycle in daemon

**Files:**
- Create: `internal/chromadb/server.go`
- Create: `internal/chromadb/server_test.go`
- Modify: `internal/daemon/daemon.go` — add chroma server field + start/stop

**Step 1: Write the failing test**

```go
// internal/chromadb/server_test.go
package chromadb_test

import (
    "testing"
    "github.com/steveyegge/gastown/internal/chromadb"
)

func TestServerConfig(t *testing.T) {
    cfg := chromadb.DefaultServerConfig()
    if cfg.Port != 8000 {
        t.Errorf("expected port 8000, got %d", cfg.Port)
    }
    if cfg.DataDir == "" {
        t.Error("expected non-empty DataDir")
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/chromadb/... -run TestServerConfig -v
```
Expected: FAIL with "undefined: DefaultServerConfig"

**Step 3: Write minimal implementation**

```go
// internal/chromadb/server.go
package chromadb

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

// ServerConfig holds configuration for the embedded Chroma server process.
type ServerConfig struct {
    Port    int
    DataDir string // where Chroma stores its data
    LogDir  string // where to write chroma.log
}

// DefaultServerConfig returns a ServerConfig using ~/.gt/chroma as data dir.
func DefaultServerConfig() ServerConfig {
    home, _ := os.UserHomeDir()
    return ServerConfig{
        Port:    8000,
        DataDir: filepath.Join(home, ".gt", "chroma"),
        LogDir:  filepath.Join(home, ".gt", "logs"),
    }
}

// Server manages the lifecycle of the Chroma subprocess.
type Server struct {
    cfg ServerConfig
    cmd *exec.Cmd
    log *log.Logger
}

// NewServer creates a new Chroma server manager.
func NewServer(cfg ServerConfig, logger *log.Logger) *Server {
    return &Server{cfg: cfg, log: logger}
}

// Start launches the Chroma server process. Non-blocking.
func (s *Server) Start(ctx context.Context) error {
    if err := os.MkdirAll(s.cfg.DataDir, 0755); err != nil {
        return fmt.Errorf("chroma: create data dir: %w", err)
    }
    if err := os.MkdirAll(s.cfg.LogDir, 0755); err != nil {
        return fmt.Errorf("chroma: create log dir: %w", err)
    }

    // Check if chroma CLI is available.
    if _, err := exec.LookPath("chroma"); err != nil {
        return fmt.Errorf("chroma binary not found in PATH — install with: pip install chromadb")
    }

    logFile, err := os.OpenFile(
        filepath.Join(s.cfg.LogDir, "chroma.log"),
        os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644,
    )
    if err != nil {
        return fmt.Errorf("chroma: open log: %w", err)
    }

    s.cmd = exec.CommandContext(ctx, "chroma", "run",
        "--path", s.cfg.DataDir,
        "--port", fmt.Sprintf("%d", s.cfg.Port),
    )
    s.cmd.Stdout = logFile
    s.cmd.Stderr = logFile

    if err := s.cmd.Start(); err != nil {
        return fmt.Errorf("chroma: start: %w", err)
    }

    s.log.Printf("Chroma server started (pid=%d, port=%d, data=%s)", s.cmd.Process.Pid, s.cfg.Port, s.cfg.DataDir)

    // Wait for server to be ready (max 10s).
    client := NewClient(fmt.Sprintf("http://localhost:%d", s.cfg.Port))
    deadline := time.Now().Add(10 * time.Second)
    for time.Now().Before(deadline) {
        if err := client.Ping(); err == nil {
            s.log.Printf("Chroma server ready")
            return nil
        }
        time.Sleep(500 * time.Millisecond)
    }
    return fmt.Errorf("chroma: server did not become ready within 10s")
}

// Stop terminates the Chroma server process.
func (s *Server) Stop() {
    if s.cmd != nil && s.cmd.Process != nil {
        _ = s.cmd.Process.Kill()
        _ = s.cmd.Wait()
        s.log.Printf("Chroma server stopped")
    }
}

// BaseURL returns the Chroma server base URL.
func (s *Server) BaseURL() string {
    return fmt.Sprintf("http://localhost:%d", s.cfg.Port)
}
```

In `internal/daemon/daemon.go`, add the chroma server field and start/stop it alongside dolt:

```go
// In Daemon struct, add:
chromaServer *chromadb.Server

// In daemon Start(), after doltServer.Start():
if !d.config.DisableChroma {
    chromaCfg := chromadb.DefaultServerConfig()
    d.chromaServer = chromadb.NewServer(chromaCfg, d.logger)
    if err := d.chromaServer.Start(ctx); err != nil {
        d.logger.Printf("WARNING: Chroma server failed to start: %v (agent memory disabled)", err)
        d.chromaServer = nil
    }
}

// In daemon Stop(), add:
if d.chromaServer != nil {
    d.chromaServer.Stop()
}
```

Add `DisableChroma bool` to `internal/daemon/config.go` (or wherever DaemonConfig is defined).

**Step 4: Run test to verify it passes**

```bash
go test ./internal/chromadb/... -v
go build ./... # verify daemon still compiles
```
Expected: all PASS, binary builds

**Step 5: Commit**

```bash
git add internal/chromadb/server.go internal/chromadb/server_test.go internal/daemon/
git commit -m "feat(chromadb): add Chroma server lifecycle in daemon"
```

---

## Phase 2: Embedding Pipeline

### Task 3: Transcript embedding on session stop

**Files:**
- Create: `internal/chromadb/embed.go`
- Create: `internal/chromadb/embed_test.go`
- Modify: `internal/hooks/` — call embed in Stop hook

**Step 1: Write the failing test**

```go
// internal/chromadb/embed_test.go
package chromadb_test

import (
    "testing"
    "github.com/steveyegge/gastown/internal/chromadb"
)

func TestChunkTranscript(t *testing.T) {
    events := []chromadb.TranscriptEvent{
        {Role: "assistant", Content: "I'll fix the auth bug.", EventType: "text"},
        {Role: "assistant", Content: "tool_call: edit file", EventType: "tool_use"},
        {Role: "assistant", Content: "I fixed it.", EventType: "text"},
    }
    chunks := chromadb.ChunkTranscript("sess-1", events)
    if len(chunks) == 0 {
        t.Fatal("expected at least one chunk")
    }
    if chunks[0].Content == "" {
        t.Error("expected non-empty content")
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/chromadb/... -run TestChunkTranscript -v
```
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/chromadb/embed.go
package chromadb

import (
    "fmt"
    "strings"
    "time"
)

// TranscriptEvent is a simplified view of an agentlog.AgentEvent for embedding.
type TranscriptEvent struct {
    Role      string
    Content   string
    EventType string
    Timestamp time.Time
}

// TranscriptChunk is one embeddable chunk from a transcript session.
type TranscriptChunk struct {
    ID        string // sessionID + "-" + index
    Content   string
    Metadata  map[string]string
}

// ChunkTranscript splits a session's events into embeddable chunks.
// Each chunk is ~500 chars of assistant content for good embedding quality.
func ChunkTranscript(sessionID string, events []TranscriptEvent) []TranscriptChunk {
    const maxChunkLen = 500
    var chunks []TranscriptChunk
    var buf strings.Builder
    chunkIdx := 0

    flush := func() {
        s := strings.TrimSpace(buf.String())
        if s == "" {
            return
        }
        chunks = append(chunks, TranscriptChunk{
            ID:      fmt.Sprintf("%s-%d", sessionID, chunkIdx),
            Content: s,
            Metadata: map[string]string{
                "session_id": sessionID,
                "chunk":      fmt.Sprintf("%d", chunkIdx),
            },
        })
        chunkIdx++
        buf.Reset()
    }

    for _, e := range events {
        if e.Role != "assistant" || e.Content == "" {
            continue
        }
        buf.WriteString(e.Content)
        buf.WriteString(" ")
        if buf.Len() >= maxChunkLen {
            flush()
        }
    }
    flush()
    return chunks
}

// EmbedTranscript upserts all chunks from a session into the "transcripts" collection.
func EmbedTranscript(client *Client, sessionID, rig, role string, events []TranscriptEvent) error {
    if err := client.EnsureCollection("transcripts"); err != nil {
        return err
    }
    chunks := ChunkTranscript(sessionID, events)
    if len(chunks) == 0 {
        return nil
    }
    docs := make([]Document, len(chunks))
    for i, c := range chunks {
        c.Metadata["rig"] = rig
        c.Metadata["role"] = role
        docs[i] = Document{ID: c.ID, Content: c.Content, Metadata: c.Metadata}
    }
    return client.Upsert("transcripts", docs)
}

// EmbedBead upserts a bead's title+description into the "beads" collection.
func EmbedBead(client *Client, beadID, title, description, rig, status, assignedTo string) error {
    if err := client.EnsureCollection("beads"); err != nil {
        return err
    }
    content := fmt.Sprintf("%s\n\n%s", title, description)
    doc := Document{
        ID:      beadID,
        Content: content,
        Metadata: map[string]string{
            "rig":         rig,
            "status":      status,
            "assigned_to": assignedTo,
        },
    }
    return client.Upsert("beads", []Document{doc})
}

// EmbedDoc upserts a documentation file chunk into the "docs" collection.
func EmbedDoc(client *Client, docID, content, rig, filePath string) error {
    if err := client.EnsureCollection("docs"); err != nil {
        return err
    }
    doc := Document{
        ID:      docID,
        Content: content,
        Metadata: map[string]string{
            "rig":       rig,
            "file_path": filePath,
        },
    }
    return client.Upsert("docs", []Document{doc})
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/chromadb/... -v
```
Expected: PASS

**Step 5: Hook into Stop hook**

In the Stop hook handler (find with `grep -rn "Stop hook\|PostExec\|stop" internal/hooks/`), after recording costs, add:

```go
// After gt costs record, add transcript embedding:
if chromaClient != nil {
    adapter := agentlog.NewAdapter("claudecode")
    events, _ := adapter.Watch(ctx, sessionID, workDir, time.Time{})
    var tevents []chromadb.TranscriptEvent
    for e := range events {
        tevents = append(tevents, chromadb.TranscriptEvent{
            Role: e.Role, Content: e.Content,
            EventType: e.EventType, Timestamp: e.Timestamp,
        })
    }
    _ = chromadb.EmbedTranscript(chromaClient, sessionID, rig, role, tevents)
}
```

**Step 6: Commit**

```bash
git add internal/chromadb/embed.go internal/chromadb/embed_test.go internal/hooks/
git commit -m "feat(chromadb): embed transcripts, beads, and docs into Chroma on session stop"
```

---

### Task 4: Context injection on polecat spawn

**Files:**
- Create: `internal/chromadb/query.go`
- Modify: `internal/polecat/` or `internal/cmd/assign.go` — inject context before spawn

**Step 1: Write the failing test**

```go
// internal/chromadb/query_test.go
package chromadb_test

import (
    "testing"
    "github.com/steveyegge/gastown/internal/chromadb"
)

func TestBuildContextSummary(t *testing.T) {
    results := chromadb.ContextResults{
        Transcripts: []chromadb.QueryResult{
            {Content: "fixed auth bug in middleware", Metadata: map[string]string{"session_id": "s1", "rig": "myapp"}},
        },
        Beads: []chromadb.QueryResult{
            {Content: "Fix JWT expiry\n\nThe token expires too early", Metadata: map[string]string{"rig": "myapp"}},
        },
        Docs: []chromadb.QueryResult{},
    }
    summary := chromadb.BuildContextSummary(results)
    if summary == "" {
        t.Error("expected non-empty context summary")
    }
    if !contains(summary, "fixed auth bug") {
        t.Errorf("expected transcript content in summary, got: %s", summary)
    }
}

func contains(s, sub string) bool {
    return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}
func containsStr(s, sub string) bool {
    for i := 0; i <= len(s)-len(sub); i++ {
        if s[i:i+len(sub)] == sub { return true }
    }
    return false
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/chromadb/... -run TestBuildContextSummary -v
```
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/chromadb/query.go
package chromadb

import (
    "fmt"
    "strings"
)

// ContextResults holds query results from all three collections.
type ContextResults struct {
    Transcripts []QueryResult
    Beads       []QueryResult
    Docs        []QueryResult
}

// QueryContext searches all three collections for context relevant to a task description.
func QueryContext(client *Client, taskDescription string) (ContextResults, error) {
    var results ContextResults
    var err error

    results.Transcripts, err = client.Query("transcripts", taskDescription, 5)
    if err != nil {
        results.Transcripts = nil // non-fatal
    }
    results.Beads, err = client.Query("beads", taskDescription, 5)
    if err != nil {
        results.Beads = nil
    }
    results.Docs, err = client.Query("docs", taskDescription, 3)
    if err != nil {
        results.Docs = nil
    }
    return results, nil
}

// BuildContextSummary formats ContextResults as a CLAUDE.md section.
func BuildContextSummary(results ContextResults) string {
    if len(results.Transcripts) == 0 && len(results.Beads) == 0 && len(results.Docs) == 0 {
        return ""
    }
    var sb strings.Builder
    sb.WriteString("## Relevant past work (from Gastown memory)\n\n")

    if len(results.Transcripts) > 0 {
        sb.WriteString("**Past sessions:**\n")
        for _, r := range results.Transcripts {
            session := r.Metadata["session_id"]
            rig := r.Metadata["rig"]
            preview := truncate(r.Content, 100)
            sb.WriteString(fmt.Sprintf("- [%s/%s] %s\n", rig, session, preview))
        }
        sb.WriteString("\n")
    }

    if len(results.Beads) > 0 {
        sb.WriteString("**Related beads:**\n")
        for _, r := range results.Beads {
            rig := r.Metadata["rig"]
            preview := truncate(firstLine(r.Content), 80)
            sb.WriteString(fmt.Sprintf("- [%s] %s\n", rig, preview))
        }
        sb.WriteString("\n")
    }

    if len(results.Docs) > 0 {
        sb.WriteString("**Relevant docs:**\n")
        for _, r := range results.Docs {
            fp := r.Metadata["file_path"]
            preview := truncate(r.Content, 80)
            sb.WriteString(fmt.Sprintf("- %s: %s\n", fp, preview))
        }
        sb.WriteString("\n")
    }

    return sb.String()
}

func truncate(s string, n int) string {
    if len(s) <= n { return s }
    return s[:n] + "…"
}

func firstLine(s string) string {
    if i := strings.Index(s, "\n"); i >= 0 { return s[:i] }
    return s
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/chromadb/... -v
```
Expected: PASS

**Step 5: Inject context in assign/spawn**

In `internal/cmd/assign.go` (or wherever `gt assign` spawns polecats), before writing the final CLAUDE.md context:

```go
if chromaClient != nil {
    ctxResults, err := chromadb.QueryContext(chromaClient, taskDescription)
    if err == nil {
        summary := chromadb.BuildContextSummary(ctxResults)
        if summary != "" {
            // Append to polecat's CLAUDE.md context section
            appendToClaudeMD(polecatWorkDir, summary)
        }
    }
}
```

**Step 6: Commit**

```bash
git add internal/chromadb/query.go internal/chromadb/query_test.go internal/cmd/assign.go
git commit -m "feat(chromadb): inject semantic context into polecat CLAUDE.md on spawn"
```

---

## Phase 3: Management REST API

### Task 5: Management API handler skeleton

**Files:**
- Create: `internal/web/manage.go`
- Create: `internal/web/manage_test.go`
- Modify: `internal/web/handler.go:459` — register `/api/manage/` and `/manage/` routes

**Step 1: Write the failing test**

```go
// internal/web/manage_test.go
package web_test

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/steveyegge/gastown/internal/web"
)

func TestManageHandlerHealth(t *testing.T) {
    h := web.NewManageHandler("test-token", "gt")
    req := httptest.NewRequest("GET", "/api/manage/health", nil)
    req.Header.Set("X-Dashboard-Token", "test-token")
    w := httptest.NewRecorder()
    h.ServeHTTP(w, req)
    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/web/... -run TestManageHandlerHealth -v
```
Expected: FAIL

**Step 3: Write minimal implementation**

```go
// internal/web/manage.go
package web

import (
    "encoding/json"
    "net/http"
    "os/exec"
    "strings"
)

// ManageHandler handles management API requests at /api/manage/.
type ManageHandler struct {
    csrfToken string
    gtPath    string
}

// NewManageHandler creates a new management API handler.
func NewManageHandler(csrfToken, gtPath string) *ManageHandler {
    if gtPath == "" {
        gtPath = "gt"
    }
    return &ManageHandler{csrfToken: csrfToken, gtPath: gtPath}
}

// ServeHTTP routes management API requests.
func (h *ManageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Validate CSRF on POST/PATCH/DELETE
    if r.Method != http.MethodGet && r.Method != http.MethodOptions {
        if r.Header.Get("X-Dashboard-Token") != h.csrfToken {
            h.sendError(w, "forbidden", http.StatusForbidden)
            return
        }
    }

    path := strings.TrimPrefix(r.URL.Path, "/api/manage")
    switch {
    case path == "/health":
        h.handleHealth(w, r)
    case path == "/polecats" && r.Method == http.MethodGet:
        h.handleListPolecats(w, r)
    case path == "/polecats" && r.Method == http.MethodPost:
        h.handleSpawnPolecat(w, r)
    case strings.HasPrefix(path, "/polecats/") && strings.HasSuffix(path, "/stop"):
        h.handleStopPolecat(w, r, extractID(path, "/polecats/", "/stop"))
    case strings.HasPrefix(path, "/polecats/") && strings.HasSuffix(path, "/stream"):
        h.handleStreamPolecat(w, r, extractID(path, "/polecats/", "/stream"))
    case path == "/beads" && r.Method == http.MethodGet:
        h.handleListBeads(w, r)
    case strings.HasPrefix(path, "/beads/") && strings.HasSuffix(path, "/assign"):
        h.handleAssignBead(w, r, extractID(path, "/beads/", "/assign"))
    case strings.HasPrefix(path, "/beads/") && strings.HasSuffix(path, "/close"):
        h.handleCloseBead(w, r, extractID(path, "/beads/", "/close"))
    case path == "/rigs" && r.Method == http.MethodGet:
        h.handleListRigs(w, r)
    case path == "/costs" && r.Method == http.MethodGet:
        h.handleCosts(w, r)
    case path == "/activity" && r.Method == http.MethodGet:
        h.handleActivity(w, r)
    case path == "/memory/search" && r.Method == http.MethodGet:
        h.handleMemorySearch(w, r)
    default:
        h.sendError(w, "not found", http.StatusNotFound)
    }
}

func (h *ManageHandler) handleHealth(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *ManageHandler) runGT(args ...string) (string, error) {
    out, err := exec.Command(h.gtPath, args...).Output()
    return string(out), err
}

func (h *ManageHandler) sendError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    _ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func extractID(path, prefix, suffix string) string {
    s := strings.TrimPrefix(path, prefix)
    return strings.TrimSuffix(s, suffix)
}
```

Register in `internal/web/handler.go` inside `NewDashboardMux`:

```go
// After line: mux.Handle("/api/", apiHandler)
manageHandler := NewManageHandler(csrfToken, "gt")
mux.Handle("/api/manage/", manageHandler)
// Serve React app for all /manage/* routes (SPA fallback)
mux.Handle("/manage/", http.HandlerFunc(serveManageSPA))
mux.Handle("/manage", http.HandlerFunc(serveManageSPA))
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/web/... -run TestManageHandlerHealth -v
go build ./...
```
Expected: PASS + binary builds

**Step 5: Commit**

```bash
git add internal/web/manage.go internal/web/manage_test.go internal/web/handler.go
git commit -m "feat(web): add management API handler skeleton at /api/manage/"
```

---

### Task 6: Polecats, beads, rigs, costs, activity endpoints

**Files:**
- Modify: `internal/web/manage.go` — implement all handlers

**Step 1: Implement handlers** (each wraps a `gt` command with `--json` flag)

```go
// Add to internal/web/manage.go

func (h *ManageHandler) handleListPolecats(w http.ResponseWriter, r *http.Request) {
    out, err := h.runGT("agents", "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleSpawnPolecat(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Task string `json:"task"`
        Rig  string `json:"rig"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.sendError(w, "bad request", http.StatusBadRequest)
        return
    }
    out, err := h.runGT("assign", req.Task, "--rig", req.Rig, "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleStopPolecat(w http.ResponseWriter, r *http.Request, id string) {
    out, err := h.runGT("signal", "stop", id, "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleListBeads(w http.ResponseWriter, r *http.Request) {
    rig := r.URL.Query().Get("rig")
    args := []string{"bead", "list", "--json"}
    if rig != "" { args = append(args, "--rig", rig) }
    out, err := h.runGT(args...)
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleAssignBead(w http.ResponseWriter, r *http.Request, id string) {
    var req struct{ AgentID string `json:"agent_id"` }
    _ = json.NewDecoder(r.Body).Decode(&req)
    out, err := h.runGT("assign", "--bead", id, "--agent", req.AgentID, "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleCloseBead(w http.ResponseWriter, r *http.Request, id string) {
    out, err := h.runGT("close", id, "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleListRigs(w http.ResponseWriter, _ *http.Request) {
    out, err := h.runGT("status", "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleCosts(w http.ResponseWriter, r *http.Request) {
    args := []string{"costs", "--json"}
    if r.URL.Query().Get("by_rig") == "1" { args = append(args, "--by-rig") }
    if r.URL.Query().Get("by_role") == "1" { args = append(args, "--by-role") }
    out, err := h.runGT(args...)
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleActivity(w http.ResponseWriter, _ *http.Request) {
    out, err := h.runGT("activity", "--json")
    h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query().Get("q")
    if q == "" {
        h.sendError(w, "q parameter required", http.StatusBadRequest)
        return
    }
    // TODO(phase2): wire to Chroma client; return empty for now
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]any{
        "transcripts": []any{},
        "beads":       []any{},
        "docs":        []any{},
    })
}

func (h *ManageHandler) proxyJSON(w http.ResponseWriter, out string, err error) {
    if err != nil {
        h.sendError(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    _, _ = w.Write([]byte(out))
}
```

**Step 2: Build and smoke test**

```bash
go build ./...
gt start &
curl -s -H "X-Dashboard-Token: $(gt config --csrf-token)" http://localhost:8080/api/manage/health
```
Expected: `{"ok":true}`

**Step 3: Commit**

```bash
git add internal/web/manage.go
git commit -m "feat(web): implement polecats/beads/rigs/costs/activity management endpoints"
```

---

### Task 7: SSE transcript streaming

**Files:**
- Create: `internal/web/sse.go`
- Create: `internal/web/sse_test.go`
- Modify: `internal/web/manage.go` — wire handleStreamPolecat

**Step 1: Write the failing test**

```go
// internal/web/sse_test.go
package web_test

import (
    "net/http/httptest"
    "testing"
    "time"
    "github.com/steveyegge/gastown/internal/web"
)

func TestSSEHeaders(t *testing.T) {
    w := httptest.NewRecorder()
    web.SetSSEHeaders(w)
    if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
        t.Errorf("expected text/event-stream, got %s", ct)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/web/... -run TestSSEHeaders -v
```
Expected: FAIL

**Step 3: Write implementation**

```go
// internal/web/sse.go
package web

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "path/filepath"
    "os"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/steveyegge/gastown/internal/agentlog"
)

// SetSSEHeaders sets the required headers for Server-Sent Events.
func SetSSEHeaders(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no")
}

// writeSSEEvent writes a single SSE event to the response writer.
func writeSSEEvent(w http.ResponseWriter, eventType string, data any) {
    b, _ := json.Marshal(data)
    fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, b)
    if f, ok := w.(http.Flusher); ok {
        f.Flush()
    }
}

// StreamTranscript streams new events from a session's transcript file via SSE.
// sessionID is the gt session name (e.g. "gt-polecat-42").
// workDir is the agent's working directory used to locate the transcript.
func StreamTranscript(ctx context.Context, w http.ResponseWriter, sessionID, workDir string) {
    SetSSEHeaders(w)

    adapter := agentlog.NewAdapter("claudecode")
    // Stream from now — only new events
    since := time.Now().Add(-30 * time.Second)
    events, err := adapter.Watch(ctx, sessionID, workDir, since)
    if err != nil {
        writeSSEEvent(w, "error", map[string]string{"message": err.Error()})
        return
    }

    writeSSEEvent(w, "connected", map[string]string{"session_id": sessionID})

    for {
        select {
        case <-ctx.Done():
            return
        case e, ok := <-events:
            if !ok {
                writeSSEEvent(w, "closed", map[string]string{"session_id": sessionID})
                return
            }
            writeSSEEvent(w, "event", map[string]any{
                "type":          e.EventType,
                "role":          e.Role,
                "content":       e.Content,
                "timestamp":     e.Timestamp,
                "input_tokens":  e.InputTokens,
                "output_tokens": e.OutputTokens,
            })
        }
    }
}
```

In `manage.go`, wire up `handleStreamPolecat`:

```go
func (h *ManageHandler) handleStreamPolecat(w http.ResponseWriter, r *http.Request, id string) {
    // Look up session workDir from gt agents output
    // For now, use a simplified approach: workDir from query param or CWD
    workDir := r.URL.Query().Get("work_dir")
    if workDir == "" {
        workDir, _ = os.Getwd()
    }
    StreamTranscript(r.Context(), w, id, workDir)
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/web/... -v
go build ./...
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/web/sse.go internal/web/sse_test.go internal/web/manage.go
git commit -m "feat(web): SSE transcript streaming for live polecat output"
```

---

## Phase 4: React Management UI

### Task 8: Vite + React scaffold

**Files:**
- Create: `ui/package.json`
- Create: `ui/vite.config.ts`
- Create: `ui/tsconfig.json`
- Create: `ui/index.html`
- Create: `ui/src/main.tsx`
- Create: `ui/src/App.tsx`

**Step 1: Scaffold the project**

```bash
cd /path/to/gastown_2
mkdir ui && cd ui
pnpm create vite@latest . --template react-ts
pnpm add react-router-dom @tanstack/react-query lucide-react
pnpm add -D tailwindcss postcss autoprefixer @types/node
pnpm dlx tailwindcss init -p
pnpm dlx shadcn@latest init
```

When shadcn asks:
- Style: Default
- Base color: Slate
- CSS variables: yes

**Step 2: Configure Vite proxy**

Edit `ui/vite.config.ts`:

```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5173,
    proxy: {
      '/api/manage': 'http://localhost:8080',
    },
  },
  build: {
    outDir: '../internal/web/static/manage',
    emptyOutDir: true,
  },
})
```

**Step 3: Install shadcn components**

```bash
cd ui
pnpm dlx shadcn@latest add button card table badge input label select textarea tabs
```

**Step 4: Verify dev server starts**

```bash
cd ui && pnpm dev
```
Expected: Vite dev server running on http://localhost:5173

**Step 5: Commit**

```bash
cd ..
git add ui/
git commit -m "feat(ui): scaffold Vite+React+TypeScript management app"
```

---

### Task 9: Layout, routing, and API client

**Files:**
- Create: `ui/src/api/client.ts`
- Create: `ui/src/components/Layout.tsx`
- Create: `ui/src/components/Sidebar.tsx`
- Modify: `ui/src/App.tsx`
- Modify: `ui/src/main.tsx`

**Step 1: API client**

```typescript
// ui/src/api/client.ts
const BASE = '/api/manage'

// Read CSRF token from meta tag injected by Go server
function getCsrfToken(): string {
  return document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content ?? ''
}

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const resp = await fetch(`${BASE}${path}`, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      'X-Dashboard-Token': getCsrfToken(),
      ...opts?.headers,
    },
  })
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({ error: resp.statusText }))
    throw new Error(err.error ?? resp.statusText)
  }
  return resp.json()
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
}

// SSE helper for transcript streaming
export function streamTranscript(sessionId: string, workDir: string, onEvent: (e: any) => void): () => void {
  const token = getCsrfToken()
  const url = `${BASE}/polecats/${sessionId}/stream?work_dir=${encodeURIComponent(workDir)}&token=${token}`
  const es = new EventSource(url)
  es.addEventListener('event', (e) => onEvent(JSON.parse(e.data)))
  es.addEventListener('error', () => es.close())
  return () => es.close()
}
```

**Step 2: Sidebar + layout**

```typescript
// ui/src/components/Sidebar.tsx
import { NavLink } from 'react-router-dom'
import { LayoutDashboard, Bot, Boxes, Server, DollarSign, Activity, Search } from 'lucide-react'

const links = [
  { to: '/manage', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/manage/polecats', label: 'Polecats', icon: Bot },
  { to: '/manage/beads', label: 'Beads', icon: Boxes },
  { to: '/manage/rigs', label: 'Rigs', icon: Server },
  { to: '/manage/costs', label: 'Costs', icon: DollarSign },
  { to: '/manage/activity', label: 'Activity', icon: Activity },
  { to: '/manage/memory', label: 'Memory', icon: Search },
]

export function Sidebar() {
  return (
    <nav className="w-56 h-screen bg-slate-900 text-slate-100 flex flex-col p-4 gap-1 fixed left-0 top-0">
      <div className="text-lg font-bold mb-6 px-2">⛽ Gastown</div>
      {links.map(({ to, label, icon: Icon, end }) => (
        <NavLink
          key={to} to={to} end={end}
          className={({ isActive }) =>
            `flex items-center gap-2 px-3 py-2 rounded text-sm transition-colors ${
              isActive ? 'bg-slate-700 text-white' : 'text-slate-400 hover:bg-slate-800 hover:text-white'
            }`
          }
        >
          <Icon size={16} /> {label}
        </NavLink>
      ))}
    </nav>
  )
}

// ui/src/components/Layout.tsx
import { Sidebar } from './Sidebar'
import { Outlet } from 'react-router-dom'

export function Layout() {
  return (
    <div className="flex">
      <Sidebar />
      <main className="ml-56 flex-1 p-6 min-h-screen bg-slate-50">
        <Outlet />
      </main>
    </div>
  )
}
```

**Step 3: App routing**

```typescript
// ui/src/App.tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Dashboard } from './pages/Dashboard'
import { Polecats } from './pages/Polecats'
import { PolecatDetail } from './pages/PolecatDetail'
import { NewPolecat } from './pages/NewPolecat'
import { Beads } from './pages/Beads'
import { Rigs } from './pages/Rigs'
import { Costs } from './pages/Costs'
import { Activity } from './pages/Activity'
import { Memory } from './pages/Memory'

const qc = new QueryClient()

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          <Route path="/manage" element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="polecats" element={<Polecats />} />
            <Route path="polecats/new" element={<NewPolecat />} />
            <Route path="polecats/:id" element={<PolecatDetail />} />
            <Route path="beads" element={<Beads />} />
            <Route path="rigs" element={<Rigs />} />
            <Route path="costs" element={<Costs />} />
            <Route path="activity" element={<Activity />} />
            <Route path="memory" element={<Memory />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
```

**Step 4: Verify routing works**

```bash
cd ui && pnpm dev
# Open http://localhost:5173/manage — should show sidebar layout
```

**Step 5: Commit**

```bash
git add ui/src/
git commit -m "feat(ui): add layout, sidebar, routing, and API client"
```

---

### Task 10: Core pages — Dashboard, Polecats, Polecat Detail

**Files:**
- Create: `ui/src/pages/Dashboard.tsx`
- Create: `ui/src/pages/Polecats.tsx`
- Create: `ui/src/pages/PolecatDetail.tsx`

**Step 1: Dashboard page**

```typescript
// ui/src/pages/Dashboard.tsx
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export function Dashboard() {
  const { data: rigs } = useQuery({ queryKey: ['rigs'], queryFn: () => api.get<any>('/rigs') })
  const { data: costs } = useQuery({ queryKey: ['costs'], queryFn: () => api.get<any>('/costs') })
  const { data: polecats } = useQuery({ queryKey: ['polecats'], queryFn: () => api.get<any[]>('/polecats') })

  const activeCount = polecats?.filter((p: any) => p.status === 'running').length ?? 0

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Dashboard</h1>
      <div className="grid grid-cols-3 gap-4 mb-8">
        <Card>
          <CardHeader><CardTitle className="text-sm text-slate-500">Active Polecats</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{activeCount}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-slate-500">Rigs</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">{Array.isArray(rigs) ? rigs.length : '—'}</p></CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="text-sm text-slate-500">Today's Spend</CardTitle></CardHeader>
          <CardContent><p className="text-3xl font-bold">${costs?.today_usd?.toFixed(2) ?? '—'}</p></CardContent>
        </Card>
      </div>
    </div>
  )
}
```

**Step 2: Polecats list page**

```typescript
// ui/src/pages/Polecats.tsx
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

export function Polecats() {
  const { data: polecats = [], isLoading } = useQuery({
    queryKey: ['polecats'],
    queryFn: () => api.get<any[]>('/polecats'),
    refetchInterval: 5000,
  })

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">Polecats</h1>
        <Link to="/manage/polecats/new"><Button>+ Spawn</Button></Link>
      </div>
      {isLoading && <p className="text-slate-400">Loading…</p>}
      <div className="space-y-2">
        {polecats.map((p: any) => (
          <Link key={p.id} to={`/manage/polecats/${p.id}`}
            className="block p-4 bg-white rounded border hover:shadow transition-shadow">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-medium">{p.id}</p>
                <p className="text-sm text-slate-500">{p.rig} · {p.task?.slice(0, 80)}</p>
              </div>
              <Badge variant={p.status === 'running' ? 'default' : 'secondary'}>{p.status}</Badge>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
```

**Step 3: Polecat detail with live transcript**

```typescript
// ui/src/pages/PolecatDetail.tsx
import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, streamTranscript } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

export function PolecatDetail() {
  const { id } = useParams<{ id: string }>()
  const qc = useQueryClient()
  const [events, setEvents] = useState<any[]>([])
  const bottomRef = useRef<HTMLDivElement>(null)

  const { data: polecat } = useQuery({
    queryKey: ['polecats', id],
    queryFn: () => api.get<any>(`/polecats`).then((ps: any[]) => ps.find(p => p.id === id)),
    refetchInterval: 3000,
  })

  // Live transcript stream
  useEffect(() => {
    if (!id || !polecat?.work_dir) return
    const stop = streamTranscript(id, polecat.work_dir, (e) => {
      setEvents(prev => [...prev, e])
      setTimeout(() => bottomRef.current?.scrollIntoView({ behavior: 'smooth' }), 50)
    })
    return stop
  }, [id, polecat?.work_dir])

  const stopMutation = useMutation({
    mutationFn: () => api.post(`/polecats/${id}/stop`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['polecats'] }),
  })

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">{id}</h1>
          <p className="text-slate-500">{polecat?.rig}</p>
        </div>
        <div className="flex gap-2">
          <Badge variant={polecat?.status === 'running' ? 'default' : 'secondary'}>{polecat?.status}</Badge>
          {polecat?.status === 'running' && (
            <Button variant="destructive" onClick={() => stopMutation.mutate()}>Stop</Button>
          )}
        </div>
      </div>

      {/* Task description */}
      {polecat?.task && (
        <div className="bg-white rounded border p-4 mb-4">
          <p className="text-sm font-medium text-slate-500 mb-1">Task</p>
          <p>{polecat.task}</p>
        </div>
      )}

      {/* Live transcript */}
      <div className="bg-slate-900 rounded p-4 h-[60vh] overflow-y-auto font-mono text-sm">
        {events.length === 0 && <p className="text-slate-500">Waiting for events…</p>}
        {events.map((e, i) => (
          <div key={i} className={`mb-2 ${e.role === 'assistant' ? 'text-green-400' : 'text-slate-400'}`}>
            {e.type === 'usage' ? (
              <span className="text-slate-600 text-xs">
                [{e.input_tokens}in / {e.output_tokens}out tokens]
              </span>
            ) : (
              <span>{e.content}</span>
            )}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
```

**Step 4: Verify in browser**

```bash
cd ui && pnpm dev
# Open http://localhost:5173/manage/polecats
# Should show list; click one for detail with live transcript
```

**Step 5: Commit**

```bash
git add ui/src/pages/
git commit -m "feat(ui): add Dashboard, Polecats list, and Polecat detail with live transcript"
```

---

### Task 11: New Polecat form with similarity search

**Files:**
- Create: `ui/src/pages/NewPolecat.tsx`

```typescript
// ui/src/pages/NewPolecat.tsx
import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

export function NewPolecat() {
  const navigate = useNavigate()
  const [task, setTask] = useState('')
  const [rig, setRig] = useState('')
  const [similar, setSimilar] = useState<any>({ transcripts: [], beads: [], docs: [] })

  const { data: rigs = [] } = useQuery({ queryKey: ['rigs'], queryFn: () => api.get<any[]>('/rigs') })

  // Live similarity search as user types
  useEffect(() => {
    if (task.length < 10) { setSimilar({ transcripts: [], beads: [], docs: [] }); return }
    const t = setTimeout(async () => {
      const res = await api.get<any>(`/memory/search?q=${encodeURIComponent(task)}`)
      setSimilar(res)
    }, 500)
    return () => clearTimeout(t)
  }, [task])

  const spawnMutation = useMutation({
    mutationFn: () => api.post('/polecats', { task, rig }),
    onSuccess: (data: any) => navigate(`/manage/polecats/${data.session_id ?? ''}`),
  })

  const hasSimilar = similar.transcripts?.length > 0 || similar.beads?.length > 0

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-bold mb-6">Spawn Polecat</h1>

      <div className="space-y-4 mb-6">
        <div>
          <Label>Task description</Label>
          <Textarea
            value={task}
            onChange={(e) => setTask(e.target.value)}
            placeholder="Describe what the agent should do…"
            rows={4}
            className="mt-1"
          />
        </div>
        <div>
          <Label>Rig</Label>
          <Select onValueChange={setRig} value={rig}>
            <SelectTrigger className="mt-1"><SelectValue placeholder="Select rig…" /></SelectTrigger>
            <SelectContent>
              {rigs.map((r: any) => (
                <SelectItem key={r.name} value={r.name}>{r.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Similarity warning */}
      {hasSimilar && (
        <Card className="mb-6 border-yellow-300 bg-yellow-50">
          <CardHeader><CardTitle className="text-sm text-yellow-800">Similar past work found</CardTitle></CardHeader>
          <CardContent className="space-y-2">
            {similar.beads?.slice(0, 3).map((b: any, i: number) => (
              <div key={i} className="text-sm">
                <Badge variant="outline" className="mr-2">bead</Badge>
                {b.content?.split('\n')[0]?.slice(0, 80)}
              </div>
            ))}
            {similar.transcripts?.slice(0, 2).map((t: any, i: number) => (
              <div key={i} className="text-sm">
                <Badge variant="outline" className="mr-2">session</Badge>
                {t.content?.slice(0, 80)}
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Button
        onClick={() => spawnMutation.mutate()}
        disabled={!task || !rig || spawnMutation.isPending}
      >
        {spawnMutation.isPending ? 'Spawning…' : 'Spawn'}
      </Button>
    </div>
  )
}
```

**Commit:**

```bash
git add ui/src/pages/NewPolecat.tsx
git commit -m "feat(ui): add New Polecat form with live Chroma similarity search"
```

---

### Task 12: Remaining pages — Beads, Rigs, Costs, Activity, Memory

**Files:**
- Create: `ui/src/pages/Beads.tsx`
- Create: `ui/src/pages/Rigs.tsx`
- Create: `ui/src/pages/Costs.tsx`
- Create: `ui/src/pages/Activity.tsx`
- Create: `ui/src/pages/Memory.tsx`

Each page follows the same pattern — `useQuery` + render table. Only Memory is shown in full as it's novel:

```typescript
// ui/src/pages/Memory.tsx
import { useState } from 'react'
import { api } from '@/api/client'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export function Memory() {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  const search = async () => {
    if (!query) return
    setLoading(true)
    const res = await api.get<any>(`/memory/search?q=${encodeURIComponent(query)}`)
    setResults(res)
    setLoading(false)
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Memory Search</h1>
      <div className="flex gap-2 mb-6">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search across all agent memory…"
          onKeyDown={(e) => e.key === 'Enter' && search()}
          className="max-w-lg"
        />
        <Button onClick={search} disabled={loading}>{loading ? 'Searching…' : 'Search'}</Button>
      </div>

      {results && (
        <div className="space-y-6">
          {['transcripts', 'beads', 'docs'].map(type => (
            results[type]?.length > 0 && (
              <div key={type}>
                <h2 className="text-lg font-semibold mb-3 capitalize">{type}</h2>
                <div className="space-y-2">
                  {results[type].map((r: any, i: number) => (
                    <Card key={i}>
                      <CardContent className="p-4">
                        <div className="flex gap-2 mb-1">
                          <Badge variant="secondary">{type.slice(0, -1)}</Badge>
                          {r.metadata?.rig && <Badge variant="outline">{r.metadata.rig}</Badge>}
                          {r.distance != null && (
                            <Badge variant="outline" className="text-slate-400">
                              {(1 - r.distance).toFixed(2)} match
                            </Badge>
                          )}
                        </div>
                        <p className="text-sm text-slate-700">{r.content?.slice(0, 200)}</p>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
            )
          ))}
        </div>
      )}
    </div>
  )
}
```

**Commit:**

```bash
git add ui/src/pages/
git commit -m "feat(ui): add Beads, Rigs, Costs, Activity, and Memory search pages"
```

---

## Phase 5: Embed UI in Go Binary

### Task 13: Embed built React app in Go server

**Files:**
- Modify: `internal/web/handler.go` — embed `static/manage/` and add SPA fallback
- Create: `Makefile` target `ui`

**Step 1: Add embed directive and SPA handler**

In `internal/web/handler.go`, add:

```go
//go:embed static/manage
var manageStaticFiles embed.FS

// serveManageSPA serves the React SPA for all /manage/* routes.
func serveManageSPA(w http.ResponseWriter, r *http.Request) {
    // Serve the React index.html for all routes (SPA client-side routing)
    data, err := manageStaticFiles.ReadFile("static/manage/index.html")
    if err != nil {
        http.Error(w, "management UI not built — run: make ui", http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    _, _ = w.Write(data)
}
```

Also serve static assets:

```go
// In NewDashboardMux, add:
manageFSys, _ := fs.Sub(manageStaticFiles, "static/manage")
manageStaticHandler := http.FileServer(http.FS(manageFSys))
mux.Handle("/manage/assets/", http.StripPrefix("/manage", manageStaticHandler))
```

**Step 2: Add Makefile target**

```makefile
# Add to Makefile:
.PHONY: ui
ui:
	cd ui && pnpm install && pnpm build
```

**Step 3: Build and verify**

```bash
make ui
go build ./...
gt start
open http://localhost:8080/manage
```
Expected: React app loads at `/manage`

**Step 4: Commit**

```bash
git add internal/web/handler.go Makefile
git commit -m "feat(web): embed built React management UI in Go binary at /manage"
```

---

## Phase 6: Wire Chroma to Memory Search

### Task 14: Connect management API memory search to Chroma

**Files:**
- Modify: `internal/web/manage.go` — wire handleMemorySearch to chromadb.Client
- Modify: `internal/web/handler.go` — pass Chroma client to ManageHandler

**Step 1: Add Chroma client to ManageHandler**

```go
// In manage.go, add chromaClient field:
type ManageHandler struct {
    csrfToken    string
    gtPath       string
    chromaClient *chromadb.Client // nil if Chroma not running
}

// Update NewManageHandler:
func NewManageHandler(csrfToken, gtPath string, chromaClient *chromadb.Client) *ManageHandler {
    ...
    return &ManageHandler{csrfToken: csrfToken, gtPath: gtPath, chromaClient: chromaClient}
}

// Update handleMemorySearch:
func (h *ManageHandler) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query().Get("q")
    if q == "" {
        h.sendError(w, "q parameter required", http.StatusBadRequest)
        return
    }
    if h.chromaClient == nil {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{
            "transcripts": []any{}, "beads": []any{}, "docs": []any{},
            "error": "Chroma not running",
        })
        return
    }
    results, err := chromadb.QueryContext(h.chromaClient, q)
    if err != nil {
        h.sendError(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]any{
        "transcripts": results.Transcripts,
        "beads":       results.Beads,
        "docs":        results.Docs,
    })
}
```

**Step 2: Pass Chroma client from daemon**

In `NewDashboardMux` (or wherever it's called from the daemon), pass the Chroma client:

```go
// handler.go: update NewDashboardMux signature
func NewDashboardMux(fetcher ConvoyFetcher, webCfg *config.WebTimeoutsConfig, chromaClient *chromadb.Client) (http.Handler, error) {
    ...
    manageHandler := NewManageHandler(csrfToken, "gt", chromaClient)
    ...
}
```

**Step 3: Build and test end-to-end**

```bash
go build ./...
gt start  # starts dolt + chroma + daemon
# Spawn a polecat to generate some transcript data
gt assign "fix the login bug" --rig myapp
# Wait for it to complete (embeds transcript on stop)
# Then search:
curl "http://localhost:8080/api/manage/memory/search?q=login+bug" \
  -H "X-Dashboard-Token: $(gt config --csrf-token)"
```
Expected: JSON with transcript/bead results related to login

**Step 4: Commit**

```bash
git add internal/web/manage.go internal/web/handler.go
git commit -m "feat(web): wire Chroma semantic search into management API memory endpoint"
```

---

## Done

At this point:
- `gt start` launches dolt + chroma + daemon
- `http://localhost:8080/manage` serves the React management UI
- All 7 pages work: Dashboard, Polecats (with live transcript), New Polecat (with similarity), Beads, Rigs, Costs, Activity, Memory
- Every polecat spawn injects relevant past context from Chroma
- Transcripts + beads are embedded into Chroma automatically

**To verify everything works:**

```bash
make ui && go build ./...
gt start
open http://localhost:8080/manage
gt assign "test task" --rig my-project
# Watch live transcript appear in browser at /manage/polecats/<id>
```
