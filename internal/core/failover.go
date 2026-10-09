package core

import "net/http"

const (
	CodeUnavailableOnFailover = "unavailable_on_failover"
	FailoverMessage           = "Not available while running on the backup server. Back when home is restored."
)

func UnavailableOnFailover(http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ErrorCode(w, http.StatusServiceUnavailable, CodeUnavailableOnFailover, FailoverMessage)
	})
}
