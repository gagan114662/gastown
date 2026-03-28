package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/chromadb"
)

var embedTranscriptCmd = &cobra.Command{
	Use:    "embed-transcript",
	Short:  "Embed session transcript into Chroma for semantic memory (called by Stop hook)",
	Hidden: true,
	RunE:   runEmbedTranscript,
}

var embedTranscriptSession string

func init() {
	embedTranscriptCmd.Flags().StringVar(&embedTranscriptSession, "session", "", "Tmux session name")
	rootCmd.AddCommand(embedTranscriptCmd)
}

// transcriptLine is the minimal struct needed to extract text content from
// Claude Code's JSONL transcript format.
type transcriptLine struct {
	Type    string               `json:"type"`
	Message *transcriptMsgBody   `json:"message,omitempty"`
}

type transcriptMsgBody struct {
	Role    string               `json:"role"`
	Content json.RawMessage      `json:"content,omitempty"`
}

// extractTextFromContent handles both string and []content-block JSON forms.
func extractTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string form first
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Try array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func runEmbedTranscript(_ *cobra.Command, _ []string) error {
	// Resolve session name
	session := embedTranscriptSession
	if session == "" {
		session = os.Getenv("GT_SESSION")
	}
	if session == "" {
		session = deriveSessionName()
	}
	if session == "" {
		// Not a Gas Town session — exit silently.
		return nil
	}

	// Resolve working directory
	workDir := os.Getenv("GT_CWD")
	if workDir == "" {
		var err error
		workDir, err = getTmuxSessionWorkDir(session)
		if err != nil || workDir == "" {
			return nil // Can't locate transcript, skip silently
		}
	}

	// Parse role/rig from session name
	role, rig, _ := parseSessionName(session)

	// Find transcript JSONL
	projectDir, err := getClaudeProjectDir(workDir)
	if err != nil {
		return nil
	}
	transcriptPath, err := findLatestTranscript(projectDir)
	if err != nil {
		return nil
	}

	// Read transcript events
	events, err := readTranscriptEvents(transcriptPath)
	if err != nil || len(events) == 0 {
		return nil
	}

	// Connect to Chroma (gracefully skip if not running)
	client := chromadb.NewClient("http://localhost:8000")
	if pingErr := client.Ping(); pingErr != nil {
		// Chroma not running — skip silently.
		return nil
	}

	// Embed transcript (errors are non-fatal; this is a best-effort hook)
	if err := chromadb.EmbedTranscript(client, session, rig, role, events); err != nil {
		fmt.Fprintf(os.Stderr, "[embed-transcript] warning: %v\n", err)
	}

	return nil
}

func readTranscriptEvents(path string) ([]chromadb.TranscriptEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []chromadb.TranscriptEvent
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg transcriptLine
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Message == nil || msg.Message.Role != "assistant" {
			continue
		}
		content := extractTextFromContent(msg.Message.Content)
		if content == "" {
			continue
		}
		events = append(events, chromadb.TranscriptEvent{
			Role:      "assistant",
			Content:   content,
			EventType: "text",
			Timestamp: time.Now(),
		})
	}
	return events, scanner.Err()
}
