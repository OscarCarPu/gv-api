package config

import (
	"os"
	"strconv"
	"time"
)

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

// getEnvFloat reads a float from the environment, falling back to defaultValue.
func getEnvFloat(key string, defaultValue float64) float64 {
	if raw := os.Getenv(key); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return v
		}
	}
	return defaultValue
}
