package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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
func StreamTranscript(ctx context.Context, w http.ResponseWriter, sessionID, workDir string) {
	SetSSEHeaders(w)

	adapter := agentlog.NewAdapter("claudecode")
	if adapter == nil {
		writeSSEEvent(w, "error", map[string]string{"message": "unsupported agent type"})
		return
	}

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
