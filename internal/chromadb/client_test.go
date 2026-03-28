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
