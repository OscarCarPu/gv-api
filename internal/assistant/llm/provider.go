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

// Usage phases. A single Suggest costs one "decide" call plus one "explore"
// call per exploratory round, so metering stays per-call while
// UsageRepository.AggregateMonth can still count interactions by "decide".
const (
	PhaseDecide    = "decide"
	PhaseExplore   = "explore"
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
	// Today is the current local date (YYYY-MM-DD), sent with the user turn so
	// "hoy"/"ayer" resolve correctly. It belongs here rather than in the system
	// prompt, which is built once at startup (and is the cached prefix).
	Today string

	// Runner lets the model run read-only queries on its own, before deciding,
	// to look up an exact name, check what data exists, or validate its final
	// query. Nil disables exploration.
	Runner QueryRunner
	// MaxQueries caps those internal reads. Zero or less disables exploration.
	MaxQueries int
}

// QueryResult is the outcome of one internal exploratory read. Err carries a
// failed query's message instead of rows so the model can see its own mistake
// and correct the SQL rather than the whole request failing.
type QueryResult struct {
	Columns   []string
	Rows      [][]any
	Truncated bool
	Err       string
}

// QueryRunner executes a read-only query on the model's behalf during Decide.
// Implemented by the assistant service over its ReadExecutor: reads run in a
// read-only, always-rolled-back transaction, so they need no user approval.
//
// A returned error aborts the whole Decide loop (context cancellation, budget
// blown); a failed query itself is reported in QueryResult.Err.
type QueryRunner interface {
	RunQuery(ctx context.Context, sql string) (QueryResult, error)
}

// DecideResult is the outcome of Decide: the final Decision plus the token
// usage of every model call it took (one "explore" entry per exploratory round,
// then the final "decide" entry).
type DecideResult struct {
	Decision Decision
	Usages   []Usage
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
	// Decision, optionally running read-only queries through in.Runner first,
	// and reports token Usage for every model call it made.
	Decide(ctx context.Context, in DecideInput) (DecideResult, error)
	// Summarize renders read-query rows into a short human-readable answer
	// (Spanish), and reports token Usage for the call.
	Summarize(ctx context.Context, in SummarizeInput) (string, Usage, error)
}
