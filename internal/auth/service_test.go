package auth

import "testing"

func TestService_Login(t *testing.T) {
	t.Run("successful flow", func(t *testing.T) {
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
}
