package auth

import "errors"

// Sentinel errors for the auth domain.
var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidToken    = errors.New("invalid token")
	ErrInvalidCode     = errors.New("invalid 2fa code")
)
