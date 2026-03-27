package ctxstack

import "math"

const DefaultMaxContextTokens = 200000

func DefaultSettings() Settings {
	return Settings{
		Enabled:   true,
		MaxTokens: DefaultMaxContextTokens,
		Budgets: BudgetConfig{
			InstructionsPct:  0.10,
			RetrievedPct:     0.20,
			CarryForwardPct:  0.30,
			ScratchpadPct:    0.10,
			OutputReservePct: 0.20,
			SafetySlackPct:   0.10,
		},
		Thresholds: Thresholds{
			WarnUsage:   0.75,
			SoftUsage:   0.85,
			HardUsage:   0.92,
			WarnEntropy: 0.60,
			SoftEntropy: 0.75,
			HardEntropy: 0.90,
		},
		Recovery: RecoveryPolicy{
			Enabled:               true,
			AutonomousAutoRecover: true,
			InteractiveWarnOnly:   true,
		},
		RuntimeOverrides: map[string]RuntimeCapabilities{},
	}
}

func (s Settings) EffectiveMaxTokens(cap RuntimeCapabilities) int {
	if cap.MaxContextTokens > 0 {
		return cap.MaxContextTokens
	}
	if s.MaxTokens > 0 {
		return s.MaxTokens
	}
	return DefaultMaxContextTokens
}

func (s Settings) Allocate(cap RuntimeCapabilities) BudgetAllocation {
	maxTokens := s.EffectiveMaxTokens(cap)
	b := s.Budgets
	if b.InstructionsPct <= 0 {
		b = DefaultSettings().Budgets
	}
	allocation := BudgetAllocation{
		MaxTokens:     maxTokens,
		Instructions:  int(math.Round(float64(maxTokens) * b.InstructionsPct)),
		Retrieved:     int(math.Round(float64(maxTokens) * b.RetrievedPct)),
		CarryForward:  int(math.Round(float64(maxTokens) * b.CarryForwardPct)),
		Scratchpad:    int(math.Round(float64(maxTokens) * b.ScratchpadPct)),
		OutputReserve: int(math.Round(float64(maxTokens) * b.OutputReservePct)),
		SafetySlack:   int(math.Round(float64(maxTokens) * b.SafetySlackPct)),
	}
	total := allocation.Instructions + allocation.Retrieved + allocation.CarryForward + allocation.Scratchpad + allocation.OutputReserve + allocation.SafetySlack
	if total != maxTokens {
		allocation.SafetySlack += maxTokens - total
	}
	return allocation
}

func BandFor(sample EntropySample, thresholds Thresholds) string {
	switch {
	case sample.ContextUsage >= thresholds.HardUsage || sample.Score >= thresholds.HardEntropy:
		return EntropyBandHard
	case sample.ContextUsage >= thresholds.SoftUsage || sample.Score >= thresholds.SoftEntropy:
		return EntropyBandSoft
	case sample.ContextUsage >= thresholds.WarnUsage || sample.Score >= thresholds.WarnEntropy:
		return EntropyBandWarn
	default:
		return EntropyBandHealthy
	}
}

func ScoreEntropy(inputs EntropyInputs) EntropySample {
	score := 0.0
	var reasons []string

	if inputs.ContextUsage > 0 {
		usageWeight := minFloat(0.45, inputs.ContextUsage*0.45)
		score += usageWeight
		if inputs.ContextUsage >= 0.75 {
			reasons = append(reasons, "high context usage")
		}
	}
	if inputs.RestartCount > 0 {
		score += minFloat(0.20, float64(inputs.RestartCount)*0.05)
		reasons = append(reasons, "repeated handoff/restart activity")
	}
	if inputs.RepeatedPromptHits > 0 {
		score += minFloat(0.10, float64(inputs.RepeatedPromptHits)*0.03)
		reasons = append(reasons, "repeated prompt patterns")
	}
	if inputs.ToolLoopHits > 0 {
		score += minFloat(0.15, float64(inputs.ToolLoopHits)*0.04)
		reasons = append(reasons, "tool loops without visible progress")
	}
	if inputs.ScratchpadEntries > 0 || inputs.ScratchpadChars > 0 {
		scratchpadWeight := minFloat(0.10, float64(inputs.ScratchpadEntries)/40.0)
		scratchpadWeight += minFloat(0.05, float64(inputs.ScratchpadChars)/12000.0)
		score += minFloat(0.15, scratchpadWeight)
		if inputs.ScratchpadEntries >= 8 || inputs.ScratchpadChars >= 2000 {
			reasons = append(reasons, "scratchpad growth without compaction")
		}
	}
	if inputs.MinutesNoProgress > 0 {
		noProgressWeight := minFloat(0.15, inputs.MinutesNoProgress/240.0)
		score += noProgressWeight
		if inputs.MinutesNoProgress >= 30 {
			reasons = append(reasons, "long no-progress window")
		}
	}

	if score > 1 {
		score = 1
	}
	return EntropySample{
		Score: score,
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
