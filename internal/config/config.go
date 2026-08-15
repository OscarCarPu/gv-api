// Package config provides the config
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DBUrl               string
	Port                string
	Password            string
	SemiprivatePassword string
	JwtSecret           string
	TotpSecret          string
	Timezone            string
	AllowedOrigins      []string

	// Domotics lights. All optional: with no LIGHTS the section is simply empty, and with
	// LIGHTS_DRIVER unset the in-memory mock is used so nothing needs a Bluetooth bridge.
	LightsDriver        string
	Lights              string
	LightsBridgeURL     string
	LightsBridgeToken   string
	LightsBridgeTimeout time.Duration
	LightsCacheTTL      time.Duration
}

func Load() (*Config, error) {
	var allowedOrigins []string
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}

	cfg := &Config{
		DBUrl:               os.Getenv("DATABASE_URL"),
		Port:                getEnv("PORT", "8080"),
		Password:            os.Getenv("PASSWORD"),
		SemiprivatePassword: os.Getenv("SEMIPRIVATE_PASSWORD"),
		JwtSecret:           os.Getenv("JWT_SECRET"),
		TotpSecret:          os.Getenv("TOTP_SECRET"),
		Timezone:            getEnv("TIMEZONE", "Europe/Madrid"),
		AllowedOrigins:      allowedOrigins,

		LightsDriver:      getEnv("LIGHTS_DRIVER", "mock"),
		Lights:            os.Getenv("LIGHTS"),
		LightsBridgeURL:   os.Getenv("LIGHTS_BRIDGE_URL"),
		LightsBridgeToken: os.Getenv("LIGHTS_BRIDGE_TOKEN"),
		// A cold BLE connect through the bridge takes ~11s; anything under ~20s aborts reads
		// that would have succeeded.
		LightsBridgeTimeout: getEnvDuration("LIGHTS_BRIDGE_TIMEOUT_MS", 20000),
		LightsCacheTTL:      getEnvDuration("LIGHTS_CACHE_MS", 2000),
	}

	var missing []string
	if cfg.DBUrl == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.Password == "" {
		missing = append(missing, "PASSWORD")
	}
	if cfg.SemiprivatePassword == "" {
		missing = append(missing, "SEMIPRIVATE_PASSWORD")
	}
	if cfg.JwtSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.TotpSecret == "" {
		missing = append(missing, "TOTP_SECRET")
	}
	if len(cfg.AllowedOrigins) == 0 {
		missing = append(missing, "ALLOWED_ORIGINS")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("required env vars not set: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// getEnvDuration reads a millisecond count from the environment, falling back to defaultMs.
func getEnvDuration(key string, defaultMs int) time.Duration {
	if raw := os.Getenv(key); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return time.Duration(defaultMs) * time.Millisecond
}
