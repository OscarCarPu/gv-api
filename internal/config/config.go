// Package config provides the config
package config

import "os"

type Config struct {
	DBUrl string
	Port  string
}

func Load() (*Config, error) {
	return &Config{
		DBUrl: os.Getenv("DATABASE_URL"),
		Port:  getEnv("PORT", "8080"),
	}, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
