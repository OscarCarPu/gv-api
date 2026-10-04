package backup

import "errors"

var (
	ErrRunning     = errors.New("a backup is already running")
	ErrInvalidName = errors.New("invalid backup name")
	ErrNotFound    = errors.New("backup not found")
)
