package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gv-api/internal/middleware"

	"github.com/go-chi/cors"
)

const origin = "https://gv.example.com"

func preflight(t *testing.T, requestHeaders string) *http.Response {
	t.Helper()

	h := cors.Handler(middleware.CORSOptions([]string{origin}))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	)

	req := httptest.NewRequest(http.MethodOptions, "/tasks/time-entries?start_time=2026-08-19T00:00:00Z", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", requestHeaders)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

// A header the browser sends but the allowlist omits makes the preflight answer
// with no Access-Control-Allow-Origin at all, which blocks every cross-origin
// call rather than only dropping that header. gv-web sends X-Device-ID on every
// request, and leaving it out took the whole deployed frontend down.
func TestCORSAllowsEveryHeaderTheWebClientSends(t *testing.T) {
	sent := []string{"content-type", "authorization", "x-device-id", "x-request-id"}

	for _, header := range sent {
		t.Run(header, func(t *testing.T) {
			resp := preflight(t, header)

			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
				t.Errorf("preflight asking for %q got Access-Control-Allow-Origin %q, want %q", header, got, origin)
			}
		})
	}

	t.Run("all at once", func(t *testing.T) {
		resp := preflight(t, "content-type,authorization,x-device-id,x-request-id")

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("got Access-Control-Allow-Origin %q, want %q", got, origin)
		}
	})
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	h := cors.Handler(middleware.CORSOptions([]string{origin}))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
	)

	req := httptest.NewRequest(http.MethodOptions, "/tasks/tasks/list-fast", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Result().Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("got Access-Control-Allow-Origin %q for a foreign origin, want none", got)
	}
}
