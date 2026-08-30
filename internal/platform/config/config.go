package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings for the Core Service, sourced from environment
// variables so the same binary behaves correctly in dev, CI and containers
// without a rebuild.
type Config struct {
	HTTPAddr    string // address the HTTP server listens on
	DatabaseURL string // postgres connection string (DSN)
	BaseURL     string // public base URL used to build short-link URLs in API responses
	Debug       bool   // enables human-readable (non-JSON) logging
	RedisAddr   string // redis address for the link cache (host:port)
	CacheTTL    time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/urlshortener?sslmode=disable"),
		BaseURL:     getEnv("BASE_URL", "http://localhost:8080"),
		Debug:       getEnv("DEBUG", "false") == "true",
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		CacheTTL:    getEnvDuration("CACHE_TTL_SECONDS", 300) * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallbackSeconds int) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(fallbackSeconds)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return time.Duration(fallbackSeconds)
	}
	return time.Duration(n)
}
