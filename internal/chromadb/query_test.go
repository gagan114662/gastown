// internal/chromadb/query_test.go
package chromadb_test

import (
	"strings"
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
	if !strings.Contains(summary, "fixed auth bug") {
		t.Errorf("expected transcript content in summary, got: %s", summary)
	}
}

func TestBuildContextSummaryEmpty(t *testing.T) {
	results := chromadb.ContextResults{}
	summary := chromadb.BuildContextSummary(results)
	if summary != "" {
		t.Errorf("expected empty summary for no results, got: %s", summary)
	}
}
