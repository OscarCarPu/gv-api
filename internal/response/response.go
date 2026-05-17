// Package response provides a set of functions to handle responses
package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("json encode error", "error", err)
	}
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// InternalError logs err and responds with 500. Use instead of Error for unexpected server-side failures.
func InternalError(w http.ResponseWriter, r *http.Request, err error, message string) {
	slog.ErrorContext(r.Context(), message, "error", err)
	Error(w, http.StatusInternalServerError, message)
}
