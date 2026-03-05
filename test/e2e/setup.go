package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"
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
	return "http://127.0.0.1:" + port
}

func getPassword() string {
	if p := os.Getenv("PASSWORD"); p != "" {
		return p
	}
	return "Abc123.."
}

func getTOTPSecret() string {
	if s := os.Getenv("TOTP_SECRET"); s != "" {
		return s
	}
	return "secret"
}

func authenticate(t *testing.T) *APIClient {
	t.Helper()
	client := NewAPIClient(t)

	tmpToken := client.Login(t, getPassword())

	code, err := totp.GenerateCode(getTOTPSecret(), time.Now())
	if err != nil {
		t.Fatalf("failed to generate totp code: %v", err)
	}

	fullToken := client.Login2FA(t, tmpToken, code)
	client.SetToken(fullToken)
	return client
}

func truncateTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, getDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "TRUNCATE habits, habit_logs, projects, tasks, todos, time_entries CASCADE")
	if err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}
