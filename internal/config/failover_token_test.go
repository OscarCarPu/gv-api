package config_test

import (
	"testing"

	"gv-api/internal/config"
)

func TestLoad_FailoverToken(t *testing.T) {
	setRequired(t)
	t.Setenv("FAILOVER_TOKEN", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FailoverToken != "" {
		t.Fatalf("unset token should stay empty, got %q", cfg.FailoverToken)
	}

	t.Setenv("FAILOVER_TOKEN", "abc123")
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FailoverToken != "abc123" {
		t.Fatalf("token = %q, want abc123", cfg.FailoverToken)
	}
}
