package core_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gv-api/internal/core"
)

func TestErrorCode(t *testing.T) {
	rec := httptest.NewRecorder()
	core.ErrorCode(rec, http.StatusServiceUnavailable, "some_code", "msg")
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 503 || body["error"] != "msg" || body["code"] != "some_code" {
		t.Errorf("got %d %v", rec.Code, body)
	}
}

func TestError_HasNoCode(t *testing.T) {
	rec := httptest.NewRecorder()
	core.Error(rec, http.StatusBadRequest, "msg")
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["code"]; ok || len(body) != 1 {
		t.Errorf("body = %v", body)
	}
}
