package core

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

func Health(db Pinger, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			slog.ErrorContext(ctx, "health check: database unreachable", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return

		}
		w.WriteHeader(http.StatusOK)
	}
}
