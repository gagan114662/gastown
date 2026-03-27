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
