package core_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gv-api/internal/core"
)

type fakePinger struct{ ping func(context.Context) error }

func (f fakePinger) Ping(ctx context.Context) error {
	return f.ping(ctx)
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name    string
		ping    func(context.Context) error
		timeout time.Duration
		want    int
	}{
		{
			name:    "database up",
			ping:    func(context.Context) error { return nil },
			timeout: time.Second,
			want:    http.StatusOK,
		},
		{
			name:    "database down",
			ping:    func(context.Context) error { return errors.New("dial tcp secret-host:5432: connection refused") },
			timeout: time.Second,
			want:    http.StatusServiceUnavailable,
		},
		{
			name: "database hangs",
			ping: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			timeout: 10 * time.Millisecond,
			want:    http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/health", nil)

			start := time.Now()
			core.Health(fakePinger{ping: tt.ping}, tt.timeout)(rec, req)

			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("took %s, want the timeout to cut it short", elapsed)
			}
			if rec.Code != tt.want {
				t.Errorf("got status %d, want %d", rec.Code, tt.want)
			}
			if strings.Contains(rec.Body.String(), "secret-host") {
				t.Errorf("body leaks the database error: %q", rec.Body.String())
			}
		})
	}
}
