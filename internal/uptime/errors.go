package uptime

import (
	"errors"

	"gv-api/internal/pipeline"
)

// Sentinel errors for the uptime domain.
var (
	// ErrNotConfigured surfaces when no pipeline database is wired up. Shared with every
	// other mart-backed domain, so one sentinel maps to one status.
	ErrNotConfigured = pipeline.ErrNotConfigured
	// ErrInvalidRange covers a from/to pair that cannot be served.
	ErrInvalidRange = errors.New("invalid range")
	// ErrUnknownDevice is a device filter that is neither of the two devices that exist.
	ErrUnknownDevice = errors.New("unknown device")
)
