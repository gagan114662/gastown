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
