// Package llm isolates the large-language-model provider behind a small,
// vendor-neutral interface so the concrete backend (Anthropic today, a local
// Ollama server later) can be swapped by configuration.
package llm

import (
	"context"
	"encoding/json"
)

// Decision kinds.
const (
	KindRead   = "read"
	KindWrite  = "write"
	KindReject = "reject"
)

// Usage phases.
const (
	PhaseDecide    = "decide"
	PhaseSummarize = "summarize"
)

// ActionCall is a structured write proposed by the model. Args is validated and
// dispatched by the assistant's ActionRegistry against the existing services.
type ActionCall struct {
	Domain    string          `json:"domain"`
	Operation string          `json:"operation"`
	Args      json.RawMessage `json:"args"`
}

// Decision is the structured output the model must return from Decide.
type Decision struct {
	Kind         string      `json:"kind"` // read | write | reject
	SQL          string      `json:"sql,omitempty"`
	Action       *ActionCall `json:"action,omitempty"`
	Explanation  string      `json:"explanation"`
	NeedsSummary bool        `json:"needs_summary,omitempty"`
	Reject       string      `json:"reject,omitempty"`
}

// Usage carries token counts for one model call so the caller can meter cost.
type Usage struct {
	Model            string
	Phase            string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// DecideInput is the request to turn natural language into a Decision.
type DecideInput struct {
	SystemPrompt string
	UserText     string
	Prior        *Decision // set on a feedback round to refine a prior suggestion
	Feedback     string    // the user's refinement text (present with Prior)
}

// SummarizeInput is the request to render read-query rows into plain language.
type SummarizeInput struct {
	SystemPrompt string
	UserText     string
	SQL          string
	Columns      []string
	Rows         [][]any
	Truncated    bool
}

// Provider is the vendor-neutral LLM surface used by the assistant.
type Provider interface {
	// Decide turns UserText (optionally refining Prior with Feedback) into a
	// Decision, and reports token Usage for the call.
	Decide(ctx context.Context, in DecideInput) (Decision, Usage, error)
	// Summarize renders read-query rows into a short human-readable answer
	// (Spanish), and reports token Usage for the call.
	Summarize(ctx context.Context, in SummarizeInput) (string, Usage, error)
}
