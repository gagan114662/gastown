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
	ID       string // sessionID + "-" + index
	Content  string
	Metadata map[string]string
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
