package auth

import (
	"testing"

	"gv-api/internal/config"
)

func setupTestService(mockTOTP TOTPValidator) *Service {
	cfg := &config.Config{
		Password:            "Abc123..",
		SemiprivatePassword: "semi-pass",
		JwtSecret:           "super-secret-test-key",
		TotpSecret:          "test-totp-secret",
	}
	return NewService(cfg, mockTOTP)
}

func TestService_Login(t *testing.T) {
	svc := setupTestService(nil)

	t.Run("main password returns tmp token", func(t *testing.T) {
		token, kind, err := svc.Login("Abc123..")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}
		if kind != "tmp" {
			t.Fatalf("expected kind 'tmp', got %q", kind)
		}
		if err := svc.ValidateToken(token, "tmp"); err != nil {
			t.Fatalf("ValidateToken(tmp) failed: %v", err)
		}
	})

	t.Run("semiprivate password returns semi token", func(t *testing.T) {
		token, kind, err := svc.Login("semi-pass")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}
		if kind != "semi" {
			t.Fatalf("expected kind 'semi', got %q", kind)
		}
		if err := svc.ValidateToken(token, "semi"); err != nil {
			t.Fatalf("ValidateToken(semi) failed: %v", err)
		}
	})

	t.Run("rejects incorrect password", func(t *testing.T) {
		_, _, err := svc.Login("incorrect password")
		if err != ErrInvalidPassword {
			t.Fatalf("Login returned wrong error: %v", err)
		}
	})
}

func TestService_Login2FA(t *testing.T) {
	t.Run("2fa success", func(t *testing.T) {
		mockTOTP := func(passcode, secret string) bool { return true }
		svc := setupTestService(mockTOTP)

		tmpToken, _, _ := svc.Login("Abc123..")

		token, err := svc.Login2FA(tmpToken, "123456")
		if err != nil {
			t.Fatalf("2FALogin failed: %v", err)
		}

		if err = svc.ValidateToken(token, "full"); err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
	})

	t.Run("2fa failure", func(t *testing.T) {
		mockTOTP := func(passcode, secret string) bool { return false }
		svc := setupTestService(mockTOTP)

		tmpToken, _, _ := svc.Login("Abc123..")

		_, err := svc.Login2FA(tmpToken, "wrong-code")
		if err != ErrInvalidCode {
			t.Fatalf("Expected ErrInvalidCode, got: %v", err)
		}
	})
}
