package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/ctxstack"
)

func openContextStore(townRoot string) (*ctxstack.Store, error) {
	if townRoot == "" {
		return nil, fmt.Errorf("town root is required")
	}
	return ctxstack.Open(townRoot)
}

func loadContextSettings(townRoot, rigName string) ctxstack.Settings {
	settings := ctxstack.DefaultSettings()
	if townRoot == "" {
		return settings
	}

	if townSettings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(townRoot)); err == nil {
		mergeContextSettings(&settings, townSettings.Context)
	}

	if rigName != "" {
		rigPath := filepath.Join(townRoot, rigName)
		if rigSettings, err := config.LoadEffectiveRigSettings(rigPath, filepath.Join(rigPath, "mayor", "rig")); err == nil {
			mergeContextSettings(&settings, rigSettings.Context)
		}
	}
	return settings
}

func mergeContextSettings(dst *ctxstack.Settings, src *config.ContextConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.Enabled = src.Enabled
	if src.MaxTokens > 0 {
		dst.MaxTokens = src.MaxTokens
	}
	if src.Budgets != nil {
		dst.Budgets = ctxstack.BudgetConfig{
			InstructionsPct:  src.Budgets.InstructionsPct,
			RetrievedPct:     src.Budgets.RetrievedPct,
			CarryForwardPct:  src.Budgets.CarryForwardPct,
			ScratchpadPct:    src.Budgets.ScratchpadPct,
			OutputReservePct: src.Budgets.OutputReservePct,
			SafetySlackPct:   src.Budgets.SafetySlackPct,
		}
	}
	if src.Thresholds != nil {
		dst.Thresholds = ctxstack.Thresholds{
			WarnUsage:   src.Thresholds.WarnUsage,
			SoftUsage:   src.Thresholds.SoftUsage,
			HardUsage:   src.Thresholds.HardUsage,
			WarnEntropy: src.Thresholds.WarnEntropy,
			SoftEntropy: src.Thresholds.SoftEntropy,
			HardEntropy: src.Thresholds.HardEntropy,
		}
	}
	if src.Recovery != nil {
		dst.Recovery = ctxstack.RecoveryPolicy{
			Enabled:               src.Recovery.Enabled,
			AutonomousAutoRecover: src.Recovery.AutonomousAutoRecover,
			InteractiveWarnOnly:   src.Recovery.InteractiveWarnOnly,
		}
	}
	if len(src.RuntimeOverrides) > 0 {
		if dst.RuntimeOverrides == nil {
			dst.RuntimeOverrides = make(map[string]ctxstack.RuntimeCapabilities)
		}
		for name, override := range src.RuntimeOverrides {
			if override == nil {
				continue
			}
			dst.RuntimeOverrides[name] = ctxstack.RuntimeCapabilities{
				NativeContextUsage: override.NativeContextUsage,
				HookSummaries:      override.HookSummaries,
				Scratchpad:         override.Scratchpad,
				EntropySignals:     override.EntropySignals,
				MaxContextTokens:   override.MaxContextTokens,
			}
		}
	}
}

func resolveRuntimeContextCapabilities(townRoot, rigName, role string) ctxstack.RuntimeCapabilities {
	return resolveRuntimeContextCapabilitiesFor(strings.TrimSpace(os.Getenv("GT_AGENT")), townRoot, rigName, role)
}

func resolveRuntimeContextCapabilitiesFor(agentOverride, townRoot, rigName, role string) ctxstack.RuntimeCapabilities {
	var rc *config.RuntimeConfig
	rigPath := ""
	if townRoot != "" && rigName != "" {
		rigPath = filepath.Join(townRoot, rigName)
	}
	if override := strings.TrimSpace(agentOverride); override != "" {
		if resolved, _, err := config.ResolveAgentConfigWithOverride(townRoot, rigPath, override); err == nil {
			rc = resolved
		}
	}
	if rc == nil && role == "crew" && os.Getenv("GT_CREW") != "" {
		rc = config.ResolveWorkerAgentConfig(os.Getenv("GT_CREW"), townRoot, rigPath)
	}
	if rc == nil && role != "" {
		rc = config.ResolveRoleAgentConfig(role, townRoot, rigPath)
	}
	if rc == nil {
		rc = config.ResolveAgentConfig(townRoot, rigPath)
	}
	if rc == nil || rc.Context == nil {
		return ctxstack.RuntimeCapabilities{}
	}
	caps := ctxstack.RuntimeCapabilities{
		NativeContextUsage: rc.Context.NativeContextUsage,
		HookSummaries:      rc.Context.HookSummaries,
		Scratchpad:         rc.Context.Scratchpad,
		EntropySignals:     rc.Context.EntropySignals,
		MaxContextTokens:   rc.Context.MaxContextTokens,
	}
	if override := strings.TrimSpace(agentOverride); override != "" {
		if settings := loadContextSettings(townRoot, rigName); settings.RuntimeOverrides != nil {
			if runtimeOverride, ok := settings.RuntimeOverrides[override]; ok {
				caps = mergeRuntimeCaps(caps, runtimeOverride)
			}
		}
	}
	return caps
}

func mergeRuntimeCaps(base, override ctxstack.RuntimeCapabilities) ctxstack.RuntimeCapabilities {
	if override.MaxContextTokens > 0 {
		base.MaxContextTokens = override.MaxContextTokens
	}
	if override.NativeContextUsage {
		base.NativeContextUsage = true
	}
	if override.HookSummaries {
		base.HookSummaries = true
	}
	if override.Scratchpad {
		base.Scratchpad = true
	}
	if override.EntropySignals {
		base.EntropySignals = true
	}
	return base
}

func syncContextMemories(store *ctxstack.Store) error {
	if store == nil {
		return nil
	}
	kvs, err := bdKvListJSON()
	if err != nil {
		return err
	}
	for key, value := range kvs {
		if !strings.HasPrefix(key, memoryKeyPrefix) {
			continue
		}
		memType, shortKey := parseMemoryKey(key)
		if err := upsertMemoryContextDoc(store, memType, shortKey, value); err != nil {
			return err
		}
	}
	return nil
}

func upsertMemoryContextDoc(store *ctxstack.Store, memType, key, value string) error {
	if store == nil {
		return nil
	}
	tags := []string{"memory", memType}
	if key != "" {
		tags = append(tags, "key:"+key)
	}
	return store.UpsertRetrievalDoc(ctxstack.RetrievalDoc{
		DocID:     memoryDocID(memType, key),
		Tier:      ctxstack.TierCold,
		Source:    "memory",
		Tags:      tags,
		Text:      strings.TrimSpace(key + "\n" + value),
		UpdatedAt: time.Now().UTC(),
		RankFeatures: map[string]any{
			"memory_type": memType,
			"key":         key,
		},
	})
}

func deleteMemoryContextDoc(store *ctxstack.Store, memType, key string) error {
	if store == nil {
		return nil
	}
	return store.DeleteRetrievalDoc(memoryDocID(memType, key))
}

func memoryDocID(memType, key string) string {
	return "memory:" + memType + ":" + key
}

func buildPrimeQuery(ctx RoleContext, hookedBead *beads.Issue) string {
	parts := []string{string(ctx.Role), ctx.Rig}
	if hookedBead != nil {
		parts = append(parts, hookedBead.ID, hookedBead.Title, hookedBead.Description)
		if fields := beads.ParseAttachmentFields(hookedBead); fields != nil {
			parts = append(parts, fields.AttachedFormula, fields.AttachedMolecule)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func renderPrimeSnapshot(snapshot *ctxstack.PrimeSnapshot) string {
	if snapshot == nil {
		return ""
	}
	var sections []string
	if snapshot.PrimarySummary != nil {
		sections = append(sections, fmt.Sprintf("## Warm Memory\n\n%s", renderSummaryBlock(*snapshot.PrimarySummary)))
	}
	if len(snapshot.Recent) > 0 {
		var lines []string
		for _, summary := range snapshot.Recent {
			lines = append(lines, "- "+renderSummaryHeadline(summary))
		}
		sections = append(sections, "## Recent Summaries\n\n"+strings.Join(lines, "\n"))
	}
	if len(snapshot.Docs) > 0 {
		var lines []string
		for _, doc := range snapshot.Docs {
			headline := strings.TrimSpace(doc.Source)
			if headline == "" {
				headline = doc.Tier
			}
			if doc.Bead != "" {
				headline += " / " + doc.Bead
			}
			lines = append(lines, fmt.Sprintf("- **%s**: %s", headline, trimContextText(doc.Text, 240)))
		}
		sections = append(sections, "## Retrieved Context\n\n"+strings.Join(lines, "\n"))
	}
	if len(sections) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n# Context Stack\n\n")
	out.WriteString(fmt.Sprintf("- Budget: instructions %d, retrieved %d, carry-forward %d, scratchpad %d, output %d, safety %d\n",
		snapshot.Budget.Instructions,
		snapshot.Budget.Retrieved,
		snapshot.Budget.CarryForward,
		snapshot.Budget.Scratchpad,
		snapshot.Budget.OutputReserve,
		snapshot.Budget.SafetySlack,
	))
	out.WriteString("\n")
	out.WriteString(strings.Join(sections, "\n\n"))
	return out.String()
}

func renderSummaryBlock(summary ctxstack.SessionSummary) string {
	var parts []string
	parts = append(parts, "**"+renderSummaryHeadline(summary)+"**")
	parts = append(parts, trimContextText(summary.Summary, 320))
	if strings.TrimSpace(summary.Changes) != "" {
		parts = append(parts, "- Changes: "+trimContextText(summary.Changes, 180))
	}
	if strings.TrimSpace(summary.Validation) != "" {
		parts = append(parts, "- Validation: "+trimContextText(summary.Validation, 180))
	}
	if strings.TrimSpace(summary.Blockers) != "" {
		parts = append(parts, "- Blockers: "+trimContextText(summary.Blockers, 180))
	}
	if strings.TrimSpace(summary.NextSteps) != "" {
		parts = append(parts, "- Next: "+trimContextText(summary.NextSteps, 180))
	}
	return strings.Join(parts, "\n")
}

func renderSummaryHeadline(summary ctxstack.SessionSummary) string {
	var parts []string
	if summary.Source != "" {
		parts = append(parts, summary.Source)
	}
	if summary.WorkBead != "" {
		parts = append(parts, summary.WorkBead)
	}
	if summary.CreatedAt.IsZero() {
		return strings.Join(parts, " / ")
	}
	parts = append(parts, summary.CreatedAt.Local().Format("2006-01-02 15:04"))
	return strings.Join(parts, " / ")
}

func trimContextText(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func currentContextSessionID() string {
	if id := os.Getenv("GT_SESSION_ID"); id != "" {
		return id
	}
	if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
		return id
	}
	return ReadPersistedSessionID()
}

func currentContextRole() string {
	role := strings.TrimSpace(os.Getenv("GT_ROLE"))
	if role == "" {
		return ""
	}
	role = config.ExtractSimpleRole(role)
	role = strings.TrimPrefix(role, "role/")
	return role
}

func currentContextRig() string {
	if rig := strings.TrimSpace(os.Getenv("GT_RIG")); rig != "" {
		return rig
	}
	return ""
}

func currentContextAgent() string {
	if agent := strings.TrimSpace(os.Getenv("GT_AGENT")); agent != "" {
		return agent
	}
	return ""
}

func currentContextBead() string {
	for _, envName := range []string{"GT_WORK_BEAD", "GT_ISSUE"} {
		if beadID := strings.TrimSpace(os.Getenv(envName)); beadID != "" {
			return beadID
		}
	}
	return ""
}

func summarizeForContext(body string) (summary, changes, blockers, nextSteps string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", "", "", ""
	}
	sections := splitMarkdownSections(body)
	summary = trimContextText(firstNonEmpty(
		sections["git state"],
		sections["hooked work"],
		body,
	), 280)
	changes = trimContextText(sections["git state"], 220)
	blockers = trimContextText(findLinesContaining(body, []string{"block", "error", "fail", "stuck"}), 220)
	nextSteps = trimContextText(firstNonEmpty(sections["hooked work"], sections["ready work"], sections["in progress"]), 220)
	return summary, changes, blockers, nextSteps
}

func splitMarkdownSections(body string) map[string]string {
	sections := make(map[string]string)
	var current string
	var lines []string
	flush := func() {
		if current == "" || len(lines) == 0 {
			return
		}
		sections[strings.ToLower(strings.TrimSpace(current))] = strings.TrimSpace(strings.Join(lines, "\n"))
		lines = nil
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.TrimPrefix(line, "## ")
			continue
		}
		lines = append(lines, line)
	}
	flush()
	return sections
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func findLinesContaining(body string, needles []string) string {
	var matches []string
	for _, line := range strings.Split(body, "\n") {
		lower := strings.ToLower(line)
		for _, needle := range needles {
			if strings.Contains(lower, needle) {
				matches = append(matches, strings.TrimSpace(line))
				break
			}
		}
	}
	return strings.Join(matches, "; ")
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func recordContextSummary(townRoot, role, rig, agent, sessionID, workBead, source, body string, tags ...string) error {
	if townRoot == "" || strings.TrimSpace(body) == "" {
		return nil
	}
	store, err := openContextStore(townRoot)
	if err != nil {
		return err
	}
	summary, changes, blockers, nextSteps := summarizeForContext(body)
	if summary == "" {
		summary = trimContextText(body, 280)
	}
	return store.PutSessionSummary(ctxstack.SessionSummary{
		SessionID: sessionID,
		Role:      role,
		Rig:       rig,
		Agent:     agent,
		WorkBead:  workBead,
		Source:    source,
		Summary:   summary,
		Changes:   changes,
		Blockers:  blockers,
		NextSteps: nextSteps,
		Tags:      dedupeStrings(tags),
		CreatedAt: time.Now().UTC(),
	})
}

func currentContextSessionSummarySource(source string) (townRoot, role, rig, agent, sessionID, workBead, kind string) {
	return detectTownRootFromCwd(),
		currentContextRole(),
		currentContextRig(),
		currentContextAgent(),
		currentContextSessionID(),
		currentContextBead(),
		source
}
