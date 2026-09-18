package tasks

import "errors"

// Sentinel errors for the tasks domain.
var (
	ErrNotFound              = errors.New("not found")
	ErrActiveTimeEntryExists = errors.New("an active time entry already exists")
	ErrCircularDependency    = errors.New("circular task dependency")
	ErrProjectCycle          = errors.New("a project cannot be moved under itself or one of its sub-projects")
	ErrParentNotFound        = errors.New("parent project not found")
)
