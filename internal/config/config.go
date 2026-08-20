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

	// Calendar. With no client id/secret the domain still mounts and answers; it simply has
	// nothing connected and never syncs, the same way lights run on a mock driver with no
	// radio. GoogleTokenKey is required as soon as credentials are present, because the
	// refresh tokens are stored encrypted and there is no plaintext fallback.
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	GoogleTokenKey     string
	// Where the OAuth callback sends the browser once the account is connected. Defaults to
	// the first allowed origin, which is gv-web in every real deployment.
	CalendarWebAppURL string
	// Push notifications. Google needs a public HTTPS address with a valid certificate; with
	// no URL configured the feature falls back to polling alone.
	CalendarWebhookURL       string
	CalendarWebhookEnabled   bool
	CalendarWatchTTL         time.Duration
	CalendarWatchRenewBefore time.Duration
	CalendarSyncInterval     time.Duration
	CalendarDebounce         time.Duration
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

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
		GoogleTokenKey:     os.Getenv("GOOGLE_TOKEN_KEY"),
		CalendarWebAppURL:  os.Getenv("CALENDAR_WEB_APP_URL"),
		CalendarWebhookURL: os.Getenv("CALENDAR_WEBHOOK_URL"),
		// Push channels last about a week in practice and cannot be renewed in place, so they
		// are replaced a day before they expire.
		CalendarWatchTTL:         getEnvDuration("CALENDAR_WATCH_TTL_MS", 7*24*60*60*1000),
		CalendarWatchRenewBefore: getEnvDuration("CALENDAR_WATCH_RENEW_BEFORE_MS", 24*60*60*1000),
		// The poll is the safety net behind the webhooks, not the main mechanism, hence 15
		// minutes rather than 1. One change in Google produces a burst of notifications, so
		// they are coalesced for a couple of seconds before syncing.
		CalendarSyncInterval: getEnvDuration("CALENDAR_SYNC_INTERVAL_MS", 15*60*1000),
		CalendarDebounce:     getEnvDuration("CALENDAR_DEBOUNCE_MS", 2000),
	}
	cfg.CalendarWebhookEnabled = getEnvBool("CALENDAR_WEBHOOK_ENABLED", cfg.CalendarWebhookURL != "")
	if cfg.CalendarWebAppURL == "" && len(allowedOrigins) > 0 {
		cfg.CalendarWebAppURL = allowedOrigins[0]
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

// getEnvBool reads a boolean from the environment, falling back to defaultValue.
func getEnvBool(key string, defaultValue bool) bool {
	if raw := os.Getenv(key); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			return v
		}
	}
	return defaultValue
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
