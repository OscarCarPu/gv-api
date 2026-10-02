package uptime

import (
	"errors"

	"gv-api/internal/pipeline"
)

// Sentinel errors for the uptime domain.
var (
	// ErrNotConfigured surfaces when no pipeline database is wired up.
	ErrNotConfigured = pipeline.ErrNotConfigured
	// ErrInvalidRange covers a from/to pair that cannot be served.
	ErrInvalidRange = errors.New("invalid range")
	// ErrUnknownDevice is a device filter matching neither device.
	ErrUnknownDevice = errors.New("unknown device")
)
