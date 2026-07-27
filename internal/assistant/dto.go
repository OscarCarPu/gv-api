package assistant

import "github.com/shopspring/decimal"

// SuggestRequest is the body of POST /assistant/suggest. On a first turn only
// Text is set; on a feedback round the client resends the opaque Token from the
// prior suggestion and Text carries the refinement.
type SuggestRequest struct {
	Text  string `json:"text"`
	Token string `json:"token,omitempty"`
}

// SuggestStep is one internal read-only query the assistant ran on its own
// while building the proposal — to find an exact name, see what data exists, or
// check its final query. These need no approval (a read cannot modify data) and
// are reported only so the user can see what informed the proposal.
type SuggestStep struct {
	SQL      string `json:"sql"`
	RowCount int    `json:"row_count"`
	Error    string `json:"error,omitempty"`
}

// SuggestResponse is the proposal shown to the user for approval. Token is the
// opaque, signed representation the client echoes back to /execute; it is empty
// when Kind is "reject".
type SuggestResponse struct {
	Kind        string        `json:"kind"` // read | write | reject
	Explanation string        `json:"explanation"`
	Query       string        `json:"query"`             // read: the SELECT; write: a human description
	Warning     string        `json:"warning,omitempty"` // e.g. destructive-write notice
	Token       string        `json:"token,omitempty"`
	Steps       []SuggestStep `json:"steps,omitempty"` // internal reads run while deciding
}

// ExecuteRequest is the body of POST /assistant/execute.
type ExecuteRequest struct {
	Token string `json:"token"`
}

// ExecuteResponse is the outcome of running an approved suggestion.
type ExecuteResponse struct {
	Kind     string `json:"kind"`                // read | write
	Summary  string `json:"summary"`             // read: readable answer; write: confirmation
	RowCount *int   `json:"row_count,omitempty"` // read only
}

// --- metering (GET /assistant/usage) ---

// MonthlyUsage is the aggregated LLM spend for a calendar month.
type MonthlyUsage struct {
	Month             string          `json:"month"`    // YYYY-MM
	Currency          string          `json:"currency"` // USD
	TotalCostUSD      decimal.Decimal `json:"total_cost_usd"`
	TotalInputTokens  int64           `json:"total_input_tokens"`
	TotalOutputTokens int64           `json:"total_output_tokens"`
	InteractionCount  int64           `json:"interaction_count"`
	ByDay             []DayUsage      `json:"by_day"`
}

// DayUsage is one day's spend within a MonthlyUsage.
type DayUsage struct {
	Date    string          `json:"date"` // YYYY-MM-DD (local calendar day)
	CostUSD decimal.Decimal `json:"cost_usd"`
	Count   int64           `json:"count"`
}
