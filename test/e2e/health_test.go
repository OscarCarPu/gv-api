package e2e

import (
	"net/http"
	"testing"
)

func TestE2E_Health(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	client := NewAPIClient(t)

	resp := client.do(t, http.MethodGet, "/health", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
