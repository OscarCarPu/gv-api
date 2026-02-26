package auth

import (
	"testing"

	"gv-api/internal/config"
)

func setupTestService(mockTOTP TOTPValidator) *Service {
	cfg := &config.Config{
		Password:   "Abc123..",
		JwtSecret:  "super-secret-test-key",
		TotpSecret: "test-totp-secret",
	}
	return NewService(cfg, mockTOTP)
}

func TestService_Login(t *testing.T) {
	svc := setupTestService(nil)

	t.Run("successful tmp token", func(t *testing.T) {
		token, err := svc.Login("Abc123..")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		err = svc.ValidateToken(token)
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
	})

	t.Run("rejects incorrect password", func(t *testing.T) {
		_, err := svc.Login("incorrect password")
		if err == nil {
			t.Fatal("Login did not fail")
		}
		if err != ErrInvalidPassword {
			t.Fatalf("Login returned wrong error: %v", err)
		}
	})
}

func TestService_Login2FA(t *testing.T) {
	t.Run("2fa success", func(t *testing.T) {
		mockTOTP := func(passcode, secret string) bool { return true }
		svc := setupTestService(mockTOTP)

		tmpToken, _ := svc.Login("Abc123..")

		token, err := svc.Login2FA(tmpToken, "123456")
		if err != nil {
			t.Fatalf("2FALogin failed: %v", err)
		}

		if err = svc.ValidateToken(token); err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
	})

	t.Run("2fa failure", func(t *testing.T) {
		mockTOTP := func(passcode, secret string) bool { return false }
		svc := setupTestService(mockTOTP)

		tmpToken, _ := svc.Login("Abc123..")

		_, err := svc.Login2FA(tmpToken, "wrong-code")
		if err != ErrInvalidCode {
			t.Fatalf("Expected ErrInvalidCode, got: %v", err)
		}
	})
}
