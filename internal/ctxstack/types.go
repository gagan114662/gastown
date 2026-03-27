package ctxstack

import "time"

const (
	TierWarm = "warm"
	TierCold = "cold"
)

const (
	EntropyBandHealthy = "healthy"
	EntropyBandWarn    = "warn"
	EntropyBandSoft    = "soft"
	EntropyBandHard    = "hard"
)

type SessionSummary struct {
	SessionID  string    `json:"session_id"`
	Role       string    `json:"role,omitempty"`
	Rig        string    `json:"rig,omitempty"`
	Agent      string    `json:"agent,omitempty"`
	WorkBead   string    `json:"work_bead,omitempty"`
	Source     string    `json:"source,omitempty"`
	Summary    string    `json:"summary"`
	Changes    string    `json:"changes,omitempty"`
	Validation string    `json:"validation,omitempty"`
	Blockers   string    `json:"blockers,omitempty"`
	NextSteps  string    `json:"next_steps,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type ScratchpadEntry struct {
	SessionID string    `json:"session_id"`
	Seq       int       `json:"seq"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type EntropySample struct {
	SessionID    string    `json:"session_id"`
	Score        float64   `json:"score"`
	Band         string    `json:"band"`
	Reasons      []string  `json:"reasons,omitempty"`
	ContextUsage float64   `json:"context_usage,omitempty"`
	Action       string    `json:"action,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type RetrievalDoc struct {
	DocID         string         `json:"doc_id"`
	Tier          string         `json:"tier"`
	Source        string         `json:"source,omitempty"`
	Rig           string         `json:"rig,omitempty"`
	Role          string         `json:"role,omitempty"`
	Bead          string         `json:"bead,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Text          string         `json:"text"`
	RankFeatures  map[string]any `json:"rank_features,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Agent         string         `json:"agent,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	SearchScore   float64        `json:"search_score,omitempty"`
	SummarySource string         `json:"summary_source,omitempty"`
}

type SummaryFilter struct {
	Role     string
	Rig      string
	Agent    string
	WorkBead string
	Source   string
	Session  string
	Since    time.Time
	Limit    int
}

type SearchOptions struct {
	Query    string
	Tier     string
	Source   string
	Rig      string
	Role     string
	Agent    string
	Bead     string
	Session  string
	Limit    int
	MaxFetch int
}

type PrimeRequest struct {
	SessionID string
	Role      string
	Rig       string
	Agent     string
	WorkBead  string
	Query     string
	MaxItems  int
}

type PrimeSnapshot struct {
	Budget         BudgetAllocation `json:"budget"`
	PrimarySummary *SessionSummary  `json:"primary_summary,omitempty"`
	Recent         []SessionSummary `json:"recent,omitempty"`
	Docs           []RetrievalDoc   `json:"docs,omitempty"`
}

type BudgetConfig struct {
	InstructionsPct  float64 `json:"instructions_pct,omitempty"`
	RetrievedPct     float64 `json:"retrieved_pct,omitempty"`
	CarryForwardPct  float64 `json:"carry_forward_pct,omitempty"`
	ScratchpadPct    float64 `json:"scratchpad_pct,omitempty"`
	OutputReservePct float64 `json:"output_reserve_pct,omitempty"`
	SafetySlackPct   float64 `json:"safety_slack_pct,omitempty"`
}

type BudgetAllocation struct {
	MaxTokens     int `json:"max_tokens"`
	Instructions  int `json:"instructions"`
	Retrieved     int `json:"retrieved"`
	CarryForward  int `json:"carry_forward"`
	Scratchpad    int `json:"scratchpad"`
	OutputReserve int `json:"output_reserve"`
	SafetySlack   int `json:"safety_slack"`
}

type Thresholds struct {
	WarnUsage   float64 `json:"warn_usage,omitempty"`
	SoftUsage   float64 `json:"soft_usage,omitempty"`
	HardUsage   float64 `json:"hard_usage,omitempty"`
	WarnEntropy float64 `json:"warn_entropy,omitempty"`
	SoftEntropy float64 `json:"soft_entropy,omitempty"`
	HardEntropy float64 `json:"hard_entropy,omitempty"`
}

type RecoveryPolicy struct {
	Enabled               bool `json:"enabled"`
	AutonomousAutoRecover bool `json:"autonomous_auto_recover"`
	InteractiveWarnOnly   bool `json:"interactive_warn_only"`
}

type RuntimeCapabilities struct {
	NativeContextUsage bool `json:"native_context_usage,omitempty"`
	HookSummaries      bool `json:"hook_summaries,omitempty"`
	Scratchpad         bool `json:"scratchpad,omitempty"`
	EntropySignals     bool `json:"entropy_signals,omitempty"`
	MaxContextTokens   int  `json:"max_context_tokens,omitempty"`
}

type Settings struct {
	Enabled          bool                           `json:"enabled"`
	MaxTokens        int                            `json:"max_tokens,omitempty"`
	Budgets          BudgetConfig                   `json:"budgets,omitempty"`
	Thresholds       Thresholds                     `json:"thresholds,omitempty"`
	Recovery         RecoveryPolicy                 `json:"recovery,omitempty"`
	RuntimeOverrides map[string]RuntimeCapabilities `json:"runtime_overrides,omitempty"`
}

type UsageSample struct {
	UsedTokens int     `json:"used_tokens"`
	MaxTokens  int     `json:"max_tokens"`
	Ratio      float64 `json:"ratio"`
	Source     string  `json:"source,omitempty"`
}

type EntropyInputs struct {
	ContextUsage       float64
	RestartCount       int
	RepeatedPromptHits int
	ToolLoopHits       int
	ScratchpadEntries  int
	ScratchpadChars    int
	MinutesNoProgress  float64
}
