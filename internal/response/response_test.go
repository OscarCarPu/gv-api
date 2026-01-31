package response

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSON_SetsContentTypeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	JSON(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	if !strings.Contains(rec.Body.String(), `"key":"value"`) {
		t.Errorf("expected body to contain key:value, got %s", rec.Body.String())
	}
}

func TestJSON_EncodesStruct(t *testing.T) {
	rec := httptest.NewRecorder()

	data := struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}{ID: 1, Name: "test"}

	JSON(rec, http.StatusCreated, data)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":1`) || !strings.Contains(body, `"name":"test"`) {
		t.Errorf("expected encoded struct, got %s", body)
	}
}

func TestError_ReturnsErrorJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	Error(rec, http.StatusBadRequest, "something went wrong")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	if !strings.Contains(rec.Body.String(), `"error":"something went wrong"`) {
		t.Errorf("expected error message in body, got %s", rec.Body.String())
	}
}

func TestError_DifferentStatusCodes(t *testing.T) {
	tests := []struct {
		status  int
		message string
	}{
		{http.StatusNotFound, "not found"},
		{http.StatusInternalServerError, "internal error"},
		{http.StatusUnauthorized, "unauthorized"},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		Error(rec, tt.status, tt.message)

		if rec.Code != tt.status {
			t.Errorf("expected status %d, got %d", tt.status, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), tt.message) {
			t.Errorf("expected message %q in body, got %s", tt.message, rec.Body.String())
		}
	}
}
