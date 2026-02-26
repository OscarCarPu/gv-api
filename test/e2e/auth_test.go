package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestE2E_Login(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := NewAPIClient(t)

	t.Run("correct password returns tmp token", func(t *testing.T) {
		token := client.Login(t, getPassword())
		if token == "" {
			t.Error("expected non-empty token")
		}
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"password": "wrong"})
		resp := client.do(t, http.MethodPost, "/login", body)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("got status %d, want 401", resp.StatusCode)
		}
	})
}

func TestE2E_Login2FA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := NewAPIClient(t)

	t.Run("valid totp code returns full token", func(t *testing.T) {
		tmpToken := client.Login(t, getPassword())

		code, err := totp.GenerateCode(getTOTPSecret(), time.Now())
		if err != nil {
			t.Fatalf("failed to generate totp code: %v", err)
		}

		fullToken := client.Login2FA(t, tmpToken, code)
		if fullToken == "" {
			t.Error("expected non-empty full token")
		}
	})

	t.Run("wrong totp code returns 401", func(t *testing.T) {
		tmpToken := client.Login(t, getPassword())

		body, _ := json.Marshal(map[string]string{"token": tmpToken, "code": "000000"})
		resp := client.do(t, http.MethodPost, "/login/2fa", body)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("got status %d, want 401", resp.StatusCode)
		}
	})

	t.Run("full token rejected at 2fa step", func(t *testing.T) {
		fullToken := authenticate(t).token

		body, _ := json.Marshal(map[string]string{"token": fullToken, "code": "123456"})
		resp := client.do(t, http.MethodPost, "/login/2fa", body)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("full token should be rejected at 2fa: got %d, want 401", resp.StatusCode)
		}
	})
}

func TestE2E_ProtectedEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := NewAPIClient(t)
	tmpToken := client.Login(t, getPassword())

	code, err := totp.GenerateCode(getTOTPSecret(), time.Now())
	if err != nil {
		t.Fatalf("failed to generate totp code: %v", err)
	}
	fullToken := client.Login2FA(t, tmpToken, code)

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"full token grants access", fullToken, http.StatusOK},
		{"tmp token is rejected", tmpToken, http.StatusUnauthorized},
		{"no token is rejected", "", http.StatusUnauthorized},
		{"invalid token is rejected", "garbage", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewAPIClient(t)
			c.SetToken(tc.token)

			var body []byte
			body, _ = json.Marshal(map[string]string{"date": "2025-01-31"})
			resp := c.do(t, http.MethodGet, "/habits?date=2025-01-31", body)
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("got %d, want %d", resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestE2E_FullAuthFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := authenticate(t)

	desc := "Auth flow habit"
	habit := client.CreateHabit(t, CreateHabitRequest{Name: "E2E Auth Test", Description: &desc})
	if habit.ID == 0 {
		t.Fatal("expected non-zero habit ID")
	}

	client.LogHabit(t, LogRequest{HabitID: habit.ID, Date: "2025-01-31", Value: 99.0})

	habits := client.GetDailyView(t, "2025-01-31")

	var found bool
	for _, h := range habits {
		if h.ID == habit.ID {
			found = true
			if h.LogValue == nil || *h.LogValue != 99.0 {
				t.Errorf("got log value %v, want 99.0", h.LogValue)
			}
			break
		}
	}
	if !found {
		t.Errorf("habit %d not found in daily view", habit.ID)
	}
}
