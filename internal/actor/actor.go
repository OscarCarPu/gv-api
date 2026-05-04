// Package actor carries per-request identity (IP, user agent, device ID,
// token kind) through context so repositories can stamp it onto audit
// records.
package actor

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type ctxKey struct{}

type Info struct {
	IP        string
	UserAgent string
	DeviceID  string
	TokenKind string
}

// WithInfo returns ctx with info attached.
func WithInfo(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

// FromContext returns the actor info attached to ctx, or a zero Info if none.
func FromContext(ctx context.Context) Info {
	info, _ := ctx.Value(ctxKey{}).(Info)
	return info
}

// WithTokenKind returns ctx with TokenKind set on the existing Info (or a new one).
func WithTokenKind(ctx context.Context, kind string) context.Context {
	info := FromContext(ctx)
	info.TokenKind = kind
	return WithInfo(ctx, info)
}

// Middleware extracts IP, User-Agent, and X-Device-ID from the request
// and attaches them to the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := Info{
			IP:        clientIP(r),
			UserAgent: r.Header.Get("User-Agent"),
			DeviceID:  r.Header.Get("X-Device-ID"),
		}
		ctx := WithInfo(r.Context(), info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
