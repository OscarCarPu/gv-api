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

	// Domotics lights. Which bulbs exist lives in the database — these only say how to reach
	// them. With LIGHTS_DRIVER unset the in-memory mock is used, so nothing needs a Bluetooth
	// adapter to run the app.
	LightsDriver  string
	LightsAdapter string
	// How long a single connect may take, discovery and GATT resolution included, and how
	// long an untouched bulb keeps the link before it is handed back to its own remote.
	LightsConnectTimeout time.Duration
	LightsIdleDisconnect time.Duration
	LightsCacheTTL       time.Duration
	// These bulbs sometimes land a step off the requested value, so the API re-applies a
	// write until it holds. 0 attempts disables it.
	LightsSettleAttempts int
	LightsSettleDelay    time.Duration
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

		LightsDriver:  getEnv("LIGHTS_DRIVER", "mock"),
		LightsAdapter: getEnv("LIGHTS_ADAPTER", "hci0"),
		// A cold connect is ~11s when BlueZ has to rediscover the bulb first; anything under
		// ~20s aborts reads that would have succeeded.
		LightsConnectTimeout: getEnvDuration("LIGHTS_CONNECT_TIMEOUT_MS", 20000),
		LightsIdleDisconnect: getEnvDuration("LIGHTS_IDLE_DISCONNECT_MS", 90000),
		LightsCacheTTL:       getEnvDuration("LIGHTS_CACHE_MS", 2000),
		LightsSettleAttempts: getEnvInt("LIGHTS_SETTLE_ATTEMPTS", 2),
		LightsSettleDelay:    getEnvDuration("LIGHTS_SETTLE_DELAY_MS", 400),
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

// getEnvInt reads an integer from the environment, falling back to defaultValue.
func getEnvInt(key string, defaultValue int) int {
	if raw := os.Getenv(key); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			return v
		}
	}
	return defaultValue
}
