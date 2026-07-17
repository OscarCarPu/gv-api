package assistant

import "errors"

// Sentinel errors for the assistant domain.
var (
	// ErrBadToken is returned when an execute token is missing, malformed, has a
	// bad signature, or has expired.
	ErrBadToken = errors.New("invalid or expired token")
	// ErrUnsafeSQL is returned when a proposed read query fails the static
	// safety checks (not a single SELECT/WITH statement).
	ErrUnsafeSQL = errors.New("unsafe sql")
	// ErrReadTooLarge is returned when a read result exceeds the row/column cap.
	ErrReadTooLarge = errors.New("result too large")
	// ErrUnsupportedAction is returned when the LLM proposes a write action that
	// is not registered.
	ErrUnsupportedAction = errors.New("unsupported action")
	// ErrInvalidAction is returned when a write action's args fail validation.
	ErrInvalidAction = errors.New("invalid action")
	// ErrActionFailed wraps a domain-service failure while executing a write
	// action (e.g. a delete blocked by dependencies). Distinct from a 500.
	ErrActionFailed = errors.New("action failed")
	// ErrProvider wraps an LLM provider failure.
	ErrProvider = errors.New("llm provider error")
)
