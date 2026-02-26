// Package config provides the config
package config

import "os"

type Config struct {
	DBUrl      string
	Port       string
	Password   string
	JwtSecret  string
	TotpSecret string
}

func Load() (*Config, error) {
	return &Config{
		DBUrl:      os.Getenv("DATABASE_URL"),
		Port:       getEnv("PORT", "8080"),
		Password:   getEnv("PASSWORD", "Abc123.."),
		JwtSecret:  getEnv("JWT_SECRET", "secret"),
		TotpSecret: getEnv("TOTP_SECRET", "secret"),
	}, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
