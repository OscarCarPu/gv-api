package e2e

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func getDBURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		t.Fatal("TEST_DB_URL is not set")
	}
	return url
}

func getBaseURL(t *testing.T) string {
	t.Helper()
	port := os.Getenv("PORT")
	if port == "" {
		t.Fatal("PORT is not set")
	}
	return "http://localhost:" + port
}

func truncateTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, getDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "TRUNCATE habits, habit_logs CASCADE")
	if err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}
