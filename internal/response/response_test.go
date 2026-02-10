package response

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	t.Run("sets content type and status", func(t *testing.T) {
		rec := httptest.NewRecorder()

		JSON(rec, http.StatusOK, map[string]string{"key": "value"})

		got := rec.Code
		want := http.StatusOK
		if got != want {
			t.Errorf("got status %d, want %d", got, want)
		}
		gotCT := rec.Header().Get("Content-Type")
		wantCT := "application/json"
		if gotCT != wantCT {
			t.Errorf("got Content-Type %q, want %q", gotCT, wantCT)
		}
		if !strings.Contains(rec.Body.String(), `"key":"value"`) {
			t.Errorf("body %q does not contain key:value", rec.Body.String())
		}
	})

	t.Run("encodes struct", func(t *testing.T) {
		rec := httptest.NewRecorder()

		data := struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}{ID: 1, Name: "test"}

		JSON(rec, http.StatusCreated, data)

		got := rec.Code
		want := http.StatusCreated
		if got != want {
			t.Errorf("got status %d, want %d", got, want)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"id":1`) || !strings.Contains(body, `"name":"test"`) {
			t.Errorf("got body %q, want encoded struct", body)
		}
	})
}

func TestError(t *testing.T) {
	t.Run("returns error as JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()

		Error(rec, http.StatusBadRequest, "something went wrong")

		got := rec.Code
		want := http.StatusBadRequest
		if got != want {
			t.Errorf("got status %d, want %d", got, want)
		}
		gotCT := rec.Header().Get("Content-Type")
		wantCT := "application/json"
		if gotCT != wantCT {
			t.Errorf("got Content-Type %q, want %q", gotCT, wantCT)
		}
		if !strings.Contains(rec.Body.String(), `"error":"something went wrong"`) {
			t.Errorf("body %q does not contain error message", rec.Body.String())
		}
	})

	t.Run("handles different status codes", func(t *testing.T) {
		cases := []struct {
			name    string
			status  int
			message string
		}{
			{"not found", http.StatusNotFound, "not found"},
			{"internal error", http.StatusInternalServerError, "internal error"},
			{"unauthorized", http.StatusUnauthorized, "unauthorized"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				Error(rec, tc.status, tc.message)

				if rec.Code != tc.status {
					t.Errorf("got status %d, want %d", rec.Code, tc.status)
				}
				if !strings.Contains(rec.Body.String(), tc.message) {
					t.Errorf("body %q does not contain %q", rec.Body.String(), tc.message)
				}
			})
		}
	})
}
