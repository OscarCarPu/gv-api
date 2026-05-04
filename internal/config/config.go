// Package config provides the config
package config

import "os"

type Config struct {
	DBUrl               string
	Port                string
	Password            string
	SemiprivatePassword string
	JwtSecret           string
	TotpSecret          string
	Timezone            string
}

func Load() (*Config, error) {
	return &Config{
		DBUrl:               os.Getenv("DATABASE_URL"),
		Port:                getEnv("PORT", "8080"),
		Password:            getEnv("PASSWORD", "Abc123.."),
		SemiprivatePassword: getEnv("SEMIPRIVATE_PASSWORD", "Abc123.."),
		JwtSecret:           getEnv("JWT_SECRET", "secret"),
		TotpSecret:          getEnv("TOTP_SECRET", "secret"),
		Timezone:            getEnv("TIMEZONE", "Europe/Madrid"),
	}, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
