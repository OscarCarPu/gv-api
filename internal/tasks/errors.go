package tasks

import "errors"

// Sentinel errors for the tasks domain.
var (
	ErrNotFound              = errors.New("not found")
	ErrActiveTimeEntryExists = errors.New("an active time entry already exists")
	ErrCircularDependency    = errors.New("circular task dependency")
)
