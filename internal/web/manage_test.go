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

func TestManageHandlerForbidden(t *testing.T) {
	h := web.NewManageHandler("secret", "gt")
	req := httptest.NewRequest("POST", "/api/manage/polecats", nil)
	// No X-Dashboard-Token header
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
