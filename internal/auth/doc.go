// Package auth provides the auth endpoints and logic.
//
// POST /login      password login; returns {token, kind}, kind being "tmp"
// (continue with /login/2fa) or "semi" (30d semiprivate token, ready to use).
// POST /login/2fa  exchanges a tmp token plus a TOTP code for a full token.
package auth
