package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type MockAuthService struct {
	mockValidateFunc func(token, kind string) error
}

func (m *MockAuthService) ValidateToken(tokenString, kind string) error {
	return m.mockValidateFunc(tokenString, kind)
}

func TestAuthMiddleware(t *testing.T) {
	fakeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mockSvc := &MockAuthService{
		mockValidateFunc: func(token, kind string) error {
			if token == "my-secret-token" {
				return nil
			}
			return errors.New("invalid token")
		},
	}

	middleware := NewMiddleware(mockSvc)

	tests := []struct {
		name           string
		token          string
		expectedStatus int
	}{
		{"ValidToken", "my-secret-token", http.StatusOK},
		{"InvalidToken", "my-secret-token-invalid", http.StatusUnauthorized},
		{"NoToken", "", http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if test.token != "" {
				req.Header.Set("Authorization", "Bearer "+test.token)
			}

			rr := httptest.NewRecorder()

			handler := middleware.Handle(fakeHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != test.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, test.expectedStatus)
			}

			if test.expectedStatus == http.StatusUnauthorized {
				contentType := rr.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("expected Content-Type application/json, got %v", contentType)
				}

				var body map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode JSON response: %v", err)
				}
				if body["error"] != "Unauthorized" {
					t.Errorf("expected error message 'Unauthorized', got %v", body["error"])
				}
			}
		})
	}
}
