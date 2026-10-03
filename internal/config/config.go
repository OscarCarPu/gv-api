// Package config loads the API's configuration from the environment.
package config

import (
	"fmt"
	"os"
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
	Location            *time.Location
	AllowedOrigins      []string

	// Domotics lights. LIGHTS_DRIVER unset uses the in-memory mock.
	LightsDriver  string
	LightsAdapter string
	// Connect budget per bulb, and how long an idle bulb keeps the link.
	LightsConnectTimeout time.Duration
	LightsIdleDisconnect time.Duration
	LightsCacheTTL       time.Duration
	LightsPollInterval   time.Duration
	// Re-apply a write until the bulb holds the value. 0 attempts disables it.
	LightsSettleAttempts int
	LightsSettleDelay    time.Duration

	// central-pipeline's database (read-only). Unset means those domains answer 503.
	PipelineDBUrl string
	// Past this age, mart numbers are reported as stale.
	PipelineStaleAfter time.Duration

	// Calendar. Without client id/secret the domain mounts but never syncs. GoogleTokenKey is
	// required once credentials are set: refresh tokens are stored encrypted.
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	GoogleTokenKey     string
	// Where the OAuth callback sends the browser. Defaults to the first allowed origin.
	CalendarWebAppURL string
	// Push notifications need a public HTTPS URL; without one, polling alone.
	CalendarWebhookURL       string
	CalendarWebhookEnabled   bool
	CalendarWatchTTL         time.Duration
	CalendarWatchRenewBefore time.Duration
	CalendarSyncInterval     time.Duration
	CalendarDebounce         time.Duration

	// Theoretical free hours per day.
	DailyCapacityHours float64

	// Health ping timeout to db
	HealthTimeout time.Duration
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
		// Each of the 3 connect retries can re-scan for 8s, so this has to cover ~24s of scanning.
		LightsConnectTimeout: getEnvDuration("LIGHTS_CONNECT_TIMEOUT_MS", 60000),
		LightsIdleDisconnect: getEnvDuration("LIGHTS_IDLE_DISCONNECT_MS", 90000),
		LightsCacheTTL:       getEnvDuration("LIGHTS_CACHE_MS", 2000),
		LightsPollInterval:   getEnvDuration("LIGHTS_POLL_MS", 60000),
		LightsSettleAttempts: getEnvInt("LIGHTS_SETTLE_ATTEMPTS", 2),
		LightsSettleDelay:    getEnvDuration("LIGHTS_SETTLE_DELAY_MS", 400),

		PipelineDBUrl: os.Getenv("PIPELINE_DATABASE_URL"),
		// Generous: dbt is not scheduled yet.
		PipelineStaleAfter: getEnvDuration("PIPELINE_STALE_AFTER_MS", 2*60*60*1000),

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
		GoogleTokenKey:     os.Getenv("GOOGLE_TOKEN_KEY"),
		CalendarWebAppURL:  os.Getenv("CALENDAR_WEB_APP_URL"),
		CalendarWebhookURL: os.Getenv("CALENDAR_WEBHOOK_URL"),
		// Channels last about a week and are replaced a day before they expire.
		CalendarWatchTTL:         getEnvDuration("CALENDAR_WATCH_TTL_MS", 7*24*60*60*1000),
		CalendarWatchRenewBefore: getEnvDuration("CALENDAR_WATCH_RENEW_BEFORE_MS", 24*60*60*1000),
		// Polling is the safety net behind webhooks. Notification bursts are coalesced before syncing.
		CalendarSyncInterval: getEnvDuration("CALENDAR_SYNC_INTERVAL_MS", 15*60*1000),
		CalendarDebounce:     getEnvDuration("CALENDAR_DEBOUNCE_MS", 2000),

		DailyCapacityHours: getEnvFloat("DAILY_CAPACITY_HOURS", 14),

		HealthTimeout: getEnvDuration("HEALTH_TIMEOUT_MS", 2000),
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

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid TIMEZONE %q: %w", cfg.Timezone, err)
	}
	cfg.Location = loc
	return cfg, nil
}
