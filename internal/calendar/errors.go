package calendar

import "errors"

// Sentinel errors for the calendar domain; the handler maps each to a status code.
var (
	ErrNotFound = errors.New("not found")
	// ErrNotConfigured is returned when GOOGLE_CLIENT_ID/SECRET are unset.
	ErrNotConfigured = errors.New("google oauth is not configured")
	// ErrNeedsReauth means the account's grant is gone; only the user can fix it.
	ErrNeedsReauth = errors.New("account needs to be reconnected")
	// ErrReadOnly is a write aimed at a calendar or an event Google would not let us change.
	ErrReadOnly = errors.New("calendar is read-only")
	// ErrConflict is a 412 from Google: the event changed elsewhere since we last read it.
	ErrConflict = errors.New("event changed in google since it was last synced")
	// ErrUpstream is Google refusing or failing a write; nothing is stored locally.
	ErrUpstream         = errors.New("google rejected the request")
	ErrInvalidState     = errors.New("invalid or expired oauth state")
	ErrInvalidRange     = errors.New("invalid time range")
	ErrInvalidScope     = errors.New("invalid scope")
	ErrCrossAccountMove = errors.New("cross-account move is not supported for this event")
)
