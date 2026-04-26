package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

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
}
