package core_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"gv-api/internal/core"
)

func TestUnavailableOnFailover(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("next handler must not be called")
			})
			rec := httptest.NewRecorder()
			core.UnavailableOnFailover(next).ServeHTTP(rec, httptest.NewRequest(method, "/domotics/lights/state", nil))

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content type = %q", ct)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["error"] != core.FailoverMessage || body["code"] != core.CodeUnavailableOnFailover {
				t.Errorf("body = %v", body)
			}
		})
	}
}

func TestUnavailableOnFailover_MountedInGroup(t *testing.T) {
	build := func(guard bool) http.Handler {
		r := chi.NewRouter()
		r.Group(func(r chi.Router) {
			if guard {
				r.Use(core.UnavailableOnFailover)
			}
			r.Get("/domotics/lights/state", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			r.Post("/domotics/lights/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		})
		return r
	}
	for _, tc := range []struct {
		method, path string
	}{{http.MethodGet, "/domotics/lights/state"}, {http.MethodPost, "/domotics/lights/x"}} {
		guarded := httptest.NewRecorder()
		build(true).ServeHTTP(guarded, httptest.NewRequest(tc.method, tc.path, nil))
		if guarded.Code != http.StatusServiceUnavailable {
			t.Errorf("guarded %s %s = %d, want 503", tc.method, tc.path, guarded.Code)
		}
		open := httptest.NewRecorder()
		build(false).ServeHTTP(open, httptest.NewRequest(tc.method, tc.path, nil))
		if open.Code != http.StatusOK {
			t.Errorf("unguarded %s %s = %d, want 200", tc.method, tc.path, open.Code)
		}
	}
}
