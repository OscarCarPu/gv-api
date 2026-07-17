// Package config provides the config
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// ModelPrice holds USD prices per 1,000,000 tokens for one model. Cache-read is
// cheaper and cache-write pricier than base input.
type ModelPrice struct {
	InputPerMTok      decimal.Decimal
	OutputPerMTok     decimal.Decimal
	CacheReadPerMTok  decimal.Decimal
	CacheWritePerMTok decimal.Decimal
}

type Config struct {
	DBUrl               string
	Port                string
	Password            string
	SemiprivatePassword string
	JwtSecret           string
	TotpSecret          string
	Timezone            string
	AllowedOrigins      []string

	// Assistant ("Voz") settings.
	AssistantProvider      string // "anthropic" | "gemini" | "stub"
	AnthropicAPIKey        string
	GeminiAPIKey           string
	AssistantModel         string
	AssistantSigningSecret string
	AssistantReadTimeoutMS int
	AssistantMaxRows       int
	// Prices maps model id -> per-MTok USD pricing (configurable, defaulted).
	Prices map[string]ModelPrice
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

		AssistantProvider:      getEnv("ASSISTANT_PROVIDER", "stub"),
		AnthropicAPIKey:        os.Getenv("ANTHROPIC_API_KEY"),
		GeminiAPIKey:           os.Getenv("GEMINI_API_KEY"),
		AssistantModel:         os.Getenv("ASSISTANT_MODEL"),
		AssistantSigningSecret: os.Getenv("ASSISTANT_SIGNING_SECRET"),
		AssistantReadTimeoutMS: getEnvInt("ASSISTANT_READ_TIMEOUT_MS", 3000),
		AssistantMaxRows:       getEnvInt("ASSISTANT_MAX_ROWS", 200),
	}

	// Default the model to the provider's cheapest/recommended when unset.
	if cfg.AssistantModel == "" {
		switch cfg.AssistantProvider {
		case "anthropic":
			cfg.AssistantModel = "claude-haiku-4-5"
		case "gemini":
			cfg.AssistantModel = "gemini-3.1-flash-lite"
		default:
			cfg.AssistantModel = "stub"
		}
	}
	// The signing secret defaults to the JWT secret (already required, random).
	if cfg.AssistantSigningSecret == "" {
		cfg.AssistantSigningSecret = cfg.JwtSecret
	}
	cfg.Prices = defaultPrices()

	var missing []string
	if cfg.DBUrl == "" {
		missing = append(missing, "DATABASE_URL")
	}
	// The provider's API key is only required when that provider is selected.
	if cfg.AssistantProvider == "anthropic" && cfg.AnthropicAPIKey == "" {
		missing = append(missing, "ANTHROPIC_API_KEY")
	}
	if cfg.AssistantProvider == "gemini" && cfg.GeminiAPIKey == "" {
		missing = append(missing, "GEMINI_API_KEY")
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

func getEnvInt(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDecimal(key, fallback string) decimal.Decimal {
	raw := getEnv(key, fallback)
	d, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		d, _ = decimal.NewFromString(fallback)
	}
	return d
}

// defaultPrices returns the per-model USD pricing table (per 1M tokens), each
// field overridable by env. Changing model or price is a config edit, never a
// code change to the cost formula. Defaults encode Anthropic Haiku 4.5 pricing.
func defaultPrices() map[string]ModelPrice {
	return map[string]ModelPrice{
		"claude-haiku-4-5": {
			InputPerMTok:      getEnvDecimal("ASSISTANT_PRICE_HAIKU_INPUT", "1.00"),
			OutputPerMTok:     getEnvDecimal("ASSISTANT_PRICE_HAIKU_OUTPUT", "5.00"),
			CacheReadPerMTok:  getEnvDecimal("ASSISTANT_PRICE_HAIKU_CACHE_READ", "0.10"),
			CacheWritePerMTok: getEnvDecimal("ASSISTANT_PRICE_HAIKU_CACHE_WRITE", "1.25"),
		},
		"gemini-3.1-flash-lite": {
			InputPerMTok:      getEnvDecimal("ASSISTANT_PRICE_GEMINI_INPUT", "0.25"),
			OutputPerMTok:     getEnvDecimal("ASSISTANT_PRICE_GEMINI_OUTPUT", "1.50"),
			CacheReadPerMTok:  getEnvDecimal("ASSISTANT_PRICE_GEMINI_CACHE_READ", "0.06"),
			CacheWritePerMTok: getEnvDecimal("ASSISTANT_PRICE_GEMINI_CACHE_WRITE", "0.25"),
		},
	}
}
