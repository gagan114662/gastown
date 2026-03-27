package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/ctxstack"
)

func TestSummarizeForContext(t *testing.T) {
	body := `## Git State
Branch: feat/context-stack
Modified: internal/cmd/prime.go

## Hooked Work
gt-123 Fix context retrieval

## Ready Work
gt-456 Follow-up validation
`
	summary, changes, blockers, nextSteps := summarizeForContext(body)
	if !strings.Contains(summary, "Branch: feat/context-stack") {
		t.Fatalf("summary = %q", summary)
	}
	if !strings.Contains(changes, "Modified") {
		t.Fatalf("changes = %q", changes)
	}
	if blockers != "" {
		t.Fatalf("blockers = %q, want empty", blockers)
	}
	if !strings.Contains(nextSteps, "gt-123") {
		t.Fatalf("nextSteps = %q", nextSteps)
	}
}

func TestRenderPrimeSnapshotIncludesSections(t *testing.T) {
	now := time.Now().UTC()
	rendered := renderPrimeSnapshot(&ctxstack.PrimeSnapshot{
		Budget: ctxstack.BudgetAllocation{
			Instructions:  100,
			Retrieved:     200,
			CarryForward:  300,
			Scratchpad:    100,
			OutputReserve: 200,
			SafetySlack:   100,
		},
		PrimarySummary: &ctxstack.SessionSummary{
			SessionID: "sess-1",
			Source:    "handoff_cycle",
			WorkBead:  "gt-123",
			Summary:   "Resume from validation and finish the endpoint.",
			CreatedAt: now,
		},
		Docs: []ctxstack.RetrievalDoc{{
			DocID:     "memory:project:endpoint",
			Source:    "memory",
			Bead:      "gt-123",
			Text:      "Endpoint needs a transaction retry loop.",
			UpdatedAt: now,
		}},
	})
	if !strings.Contains(rendered, "# Context Stack") {
		t.Fatalf("rendered output missing header: %s", rendered)
	}
	if !strings.Contains(rendered, "## Warm Memory") {
		t.Fatalf("rendered output missing warm memory section: %s", rendered)
	}
	if !strings.Contains(rendered, "## Retrieved Context") {
		t.Fatalf("rendered output missing retrieved context section: %s", rendered)
	}
}
