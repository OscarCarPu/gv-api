// Package middleware provides the HTTP middleware not specific to any domain.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
)

type contextKey string

const requestIDKey contextKey = "requestID"

// RequestID stamps each request with the client's X-Request-ID or a random one,
// returns it in the response header, and puts it in the context for LogHandler.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LogHandler adds the request id to records logged with a request context, so a
// client reporting an error can be matched against the log line for it.
type LogHandler struct{ slog.Handler }

func (h LogHandler) Handle(ctx context.Context, record slog.Record) error {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		record.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, record)
}
