package core

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ParseIDParam reads the "id" URL parameter as a positive int32.
func ParseIDParam(r *http.Request, entity string) (int32, error) {
	s := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(s, 10, 32)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s id", entity)
	}
	return int32(id), nil
}
