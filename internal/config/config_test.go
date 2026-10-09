package config_test

import (
	"os"
	"strings"
	"testing"

	"gv-api/internal/config"
)

func setRequired(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"DATABASE_URL": "postgres://x", "PASSWORD": "p", "SEMIPRIVATE_PASSWORD": "s",
		"JWT_SECRET": "j", "TOTP_SECRET": "t", "ALLOWED_ORIGINS": "http://localhost",
	} {
		t.Setenv(k, v)
	}
}

func TestLoad_FailoverSide(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		set     bool
		want    string
		onAWS   bool
		wantErr bool
	}{
		{name: "unset", want: "home"},
		{name: "home", set: true, value: "home", want: "home"},
		{name: "aws", set: true, value: "aws", want: "aws", onAWS: true},
		{name: "case and spaces", set: true, value: "AWS ", want: "aws", onAWS: true},
		{name: "invalid", set: true, value: "foo", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv("FAILOVER_SIDE", tc.value)
			if !tc.set {
				os.Unsetenv("FAILOVER_SIDE")
			}
			cfg, err := config.Load()
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "FAILOVER_SIDE") {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.FailoverSide != tc.want || cfg.OnFailover() != tc.onAWS {
				t.Errorf("side = %q, onFailover = %v", cfg.FailoverSide, cfg.OnFailover())
			}
		})
	}
}
