package auth

import (
	"net/http"
	"strings"
)

type TokenValidator interface {
	ValidateToken(tokenString string) error
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
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		err := m.authService.ValidateToken(token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
