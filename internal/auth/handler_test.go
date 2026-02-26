package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func assertStatus(t testing.TB, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("got status %d, want %d", got, want)
	}
}

func assertBodyContains(t testing.TB, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("body %q does not contain %q", body, want)
	}
}

func TestHandler_Login(t *testing.T) {
	svc := setupTestService(nil)
	h := NewHandler(svc)

	t.Run("returns token on valid password", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"password": "Abc123.."})
		req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
		rr := httptest.NewRecorder()

		h.Login(rr, req)

		assertStatus(t, rr.Code, http.StatusOK)
		assertBodyContains(t, rr.Body.String(), "token")
	})

	errorCases := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "returns 400 for invalid JSON",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid Request",
		},
		{
			name:       "returns 401 for wrong password",
			body:       `{"password": "wrong"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   ErrInvalidPassword.Error(),
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()

			h.Login(rr, req)

			assertStatus(t, rr.Code, tc.wantStatus)
			assertBodyContains(t, rr.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_Login2FA(t *testing.T) {
	mockTOTP := func(passcode, secret string) bool { return true }
	svc := setupTestService(mockTOTP)
	h := NewHandler(svc)

	t.Run("returns full token on valid tmp token and code", func(t *testing.T) {
		tmpToken, _ := svc.Login("Abc123..")

		body, _ := json.Marshal(map[string]string{"token": tmpToken, "code": "123456"})
		req := httptest.NewRequest(http.MethodPost, "/login/2fa", bytes.NewBuffer(body))
		rr := httptest.NewRecorder()

		h.Login2FA(rr, req)

		assertStatus(t, rr.Code, http.StatusOK)
		assertBodyContains(t, rr.Body.String(), "token")
	})

	errorCases := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "returns 400 for invalid JSON",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid Request",
		},
		{
			name:       "returns 401 for invalid tmp token",
			body:       `{"token": "bad-token", "code": "123456"}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   ErrInvalidToken.Error(),
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/login/2fa", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()

			h.Login2FA(rr, req)

			assertStatus(t, rr.Code, tc.wantStatus)
			assertBodyContains(t, rr.Body.String(), tc.wantBody)
		})
	}
}
