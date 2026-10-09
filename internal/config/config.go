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
	FailoverSide        string

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

	// Domotics printers. Unset PRUSALINK_HOST leaves telemetry and files unconfigured.
	PrinterRTSP                string
	PrusaLinkHost              string
	PrusaLinkUser              string
	PrusaLinkPassword          string
	PrusaLinkAPIKey            string
	PrusaLinkStorage           string
	PrinterRecordingsDir       string
	PrinterUploadsDir          string
	PrinterRecordingMaxMinutes int
	PrinterRecordingsMaxGB     float64
	PrinterRecordingVideoCodec string
	PrinterRecordingOverlay    bool
	PrinterRecordingFont       string

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

	// Backups
	BackupDir        string
	BackupKeepHourly time.Duration
	BackupKeepDaily  time.Duration
	BackupInterval   time.Duration
	BackupS3Bucket   string
}

func (c *Config) OnFailover() bool { return c.FailoverSide == "aws" }

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
		FailoverSide:        strings.ToLower(strings.TrimSpace(getEnv("FAILOVER_SIDE", "home"))),

		LightsDriver:  getEnv("LIGHTS_DRIVER", "mock"),
		LightsAdapter: getEnv("LIGHTS_ADAPTER", "hci0"),
		// Each of the 3 connect retries can re-scan for 8s, so this has to cover ~24s of scanning.
		LightsConnectTimeout: getEnvDuration("LIGHTS_CONNECT_TIMEOUT_MS", 60000),
		LightsIdleDisconnect: getEnvDuration("LIGHTS_IDLE_DISCONNECT_MS", 90000),
		LightsCacheTTL:       getEnvDuration("LIGHTS_CACHE_MS", 2000),
		LightsPollInterval:   getEnvDuration("LIGHTS_POLL_MS", 60000),
		LightsSettleAttempts: getEnvInt("LIGHTS_SETTLE_ATTEMPTS", 2),
		LightsSettleDelay:    getEnvDuration("LIGHTS_SETTLE_DELAY_MS", 400),

		PrinterRTSP:                getEnv("PRINTER_RTSP_URL", "rtsp://192.168.1.211/live"),
		PrusaLinkHost:              strings.TrimRight(os.Getenv("PRUSALINK_HOST"), "/"),
		PrusaLinkUser:              os.Getenv("PRUSALINK_USER"),
		PrusaLinkPassword:          os.Getenv("PRUSALINK_PASSWORD"),
		PrusaLinkAPIKey:            os.Getenv("PRUSALINK_API_KEY"),
		PrusaLinkStorage:           os.Getenv("PRUSALINK_STORAGE"),
		PrinterRecordingsDir:       getEnv("PRINTER_RECORDINGS_DIR", "/data/recordings"),
		PrinterUploadsDir:          getEnv("PRINTER_UPLOADS_DIR", os.TempDir()+"/gv-print-uploads"),
		PrinterRecordingMaxMinutes: getEnvInt("PRINTER_RECORDING_MAX_MINUTES", 30*60),
		PrinterRecordingsMaxGB:     getEnvFloat("PRINTER_RECORDINGS_MAX_GB", 30),
		PrinterRecordingVideoCodec: getEnv("PRINTER_RECORDING_VIDEO_CODEC", "copy"),
		PrinterRecordingOverlay:    getEnvBool("PRINTER_RECORDING_OVERLAY", true),
		PrinterRecordingFont:       os.Getenv("PRINTER_RECORDING_FONT"),

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

		BackupDir:        getEnv("BACKUP_DIR", "/backups"),
		BackupKeepHourly: time.Duration(getEnvInt("BACKUP_KEEP_HOURLY_DAYS", 2)) * 24 * time.Hour,
		BackupKeepDaily:  time.Duration(getEnvInt("BACKUP_KEEP_DAILY_DAYS", 30)) * 24 * time.Hour,
		BackupInterval:   getEnvDuration("BACKUP_INTERVAL_MS", 60*60*1000),
		BackupS3Bucket:   os.Getenv("BACKUP_S3_BUCKET"),
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
	if cfg.BackupKeepHourly <= 0 || cfg.BackupKeepDaily < cfg.BackupKeepHourly {
		return nil, fmt.Errorf("BACKUP_KEEP_HOURLY_DAYS must be at least 1 and at most BACKUP_KEEP_DAILY_DAYS")
	}

	if cfg.FailoverSide != "home" && cfg.FailoverSide != "aws" {
		return nil, fmt.Errorf("invalid FAILOVER_SIDE %q: must be home or aws", cfg.FailoverSide)
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid TIMEZONE %q: %w", cfg.Timezone, err)
	}
	cfg.Location = loc
	return cfg, nil
}
