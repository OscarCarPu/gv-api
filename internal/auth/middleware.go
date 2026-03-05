package auth

import (
	"net/http"
	"strings"

	"gv-api/internal/response"
)

type TokenValidator interface {
	ValidateToken(tokenString, kind string) error
}

type Middleware struct {
	authService TokenValidator
}

func NewMiddleware(authService TokenValidator) *Middleware {
	return &Middleware{authService: authService}
}

func (m *Middleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			response.Error(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			response.Error(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		err := m.authService.ValidateToken(token, "full")
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}
