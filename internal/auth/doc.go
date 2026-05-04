// Package auth provides the auth endpoints and logic
//
// POST /auth/login - Login with password; returns {token, kind} where kind is
//   "tmp" (use with /auth/2fa) or "semi" (30d semiprivate token, ready to use)
// POST /auth/2fa - Login with 2fa to get token
package auth
