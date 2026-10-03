package auth

import (
	"net/http"
	"strings"

	"gv-api/internal/core"
)

type TokenValidator interface {
	ValidateToken(tokenString, kind string) error
}

type Middleware struct {
	authService TokenValidator
	kinds       []string
}

func NewMiddleware(authService TokenValidator, kinds ...string) *Middleware {
	return &Middleware{authService: authService, kinds: kinds}
}

func (m *Middleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			core.Error(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			core.Error(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		for _, kind := range m.kinds {
			if m.authService.ValidateToken(token, kind) == nil {
				next.ServeHTTP(w, r)
				return
			}
		}

		core.Error(w, http.StatusUnauthorized, "Unauthorized")
	})
}
