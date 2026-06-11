package plan

import "errors"

// Sentinel errors for the plan domain. Repository errors signal data-layer
// outcomes; the rest are business-rule violations raised by the service.
var (
	ErrNotFound         = errors.New("not found")
	ErrTaskNotFound     = errors.New("task not found")
	ErrInvalidTimeRange = errors.New("ended_at must be after started_at")
	ErrLabelRequired    = errors.New("label or task_id is required")
	ErrLabelTooLong     = errors.New("label must be at most 200 characters")
	ErrOverlap          = errors.New("plan block overlaps with an existing one")
)
