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

	transcripts, err := client.Query("transcripts", taskDescription, 5)
	if err != nil {
		transcripts = nil // non-fatal
	}
	results.Transcripts = transcripts

	beads, err := client.Query("beads", taskDescription, 5)
	if err != nil {
		beads = nil
	}
	results.Beads = beads

	docs, err := client.Query("docs", taskDescription, 3)
	if err != nil {
		docs = nil
	}
	results.Docs = docs

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
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}
