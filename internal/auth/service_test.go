package auth

import "testing"

func TestService_Login(t *testing.T) {
	t.Run("successful tmp token", func(t *testing.T) {
		token, err := Login("Abc123..")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		err = ValidateToken(token)
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
	})

	t.Run("rejects incorrect password", func(t *testing.T) {
		_, err := Login("incorrect password")
		if err == nil {
			t.Fatal("Login did not fail")
		}
		if err != ErrInvalidPassword {
			t.Fatalf("Login returned wrong error: %v", err)
		}
	})

	t.Run("rejects fake tokens", func(t *testing.T) {
		err := ValidateToken("fake token")
		if err == nil {
			t.Fatal("ValidateToken did not fail")
		}
		if err != ErrInvalidToken {
			t.Fatalf("ValidateToken returned wrong error: %v", err)
		}
	})

	t.Run("2fa success", func(t *testing.T) {
		tmpToken, err := Login("Abc123..")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		validateTOTP = func(_, _ string) bool { return true }
		token, err := Login2FA(tmpToken, "123456")
		if err != nil {
			t.Fatalf("2FALogin failed: %v", err)
		}

		err = ValidateToken(token)
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
	})

	t.Run("2fa failure", func(t *testing.T) {
		tmpToken, err := Login("Abc123..")
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		validateTOTP = func(_, _ string) bool { return false }
		token, err := Login2FA(tmpToken, "123456")
		if err == nil {
			t.Fatalf("2FALogin did not fail")
		}
		if err != ErrInvalidCode {
			t.Fatalf("2FALogin returned wrong error: %v", err)
		}

		err = ValidateToken(token)
		if err != ErrInvalidToken {
			t.Fatalf("2FALogin returned wrong error: %v", err)
		}
	})
}
