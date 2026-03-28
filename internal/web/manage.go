package web

import (
	"encoding/json"
	"net/http"
	"os"
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
	// Validate CSRF token on mutating requests.
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

func (h *ManageHandler) handleStreamPolecat(w http.ResponseWriter, r *http.Request, id string) {
	workDir := r.URL.Query().Get("work_dir")
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	StreamTranscript(r.Context(), w, id, workDir)
}

func (h *ManageHandler) handleListBeads(w http.ResponseWriter, r *http.Request) {
	rig := r.URL.Query().Get("rig")
	args := []string{"bead", "list", "--json"}
	if rig != "" {
		args = append(args, "--rig", rig)
	}
	out, err := h.runGT(args...)
	h.proxyJSON(w, out, err)
}

func (h *ManageHandler) handleAssignBead(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
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
	if r.URL.Query().Get("by_rig") == "1" {
		args = append(args, "--by-rig")
	}
	if r.URL.Query().Get("by_role") == "1" {
		args = append(args, "--by-role")
	}
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
	// Placeholder — wired to Chroma in Task 14.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"transcripts": []any{},
		"beads":       []any{},
		"docs":        []any{},
	})
}

func (h *ManageHandler) runGT(args ...string) (string, error) {
	out, err := exec.Command(h.gtPath, args...).Output()
	return string(out), err
}

func (h *ManageHandler) proxyJSON(w http.ResponseWriter, out string, err error) {
	if err != nil {
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(out))
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
