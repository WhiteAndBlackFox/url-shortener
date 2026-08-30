package config

import (
	"os"
	"strconv"
	"time"
)

// CoreConfig holds runtime settings for the Core Service, sourced from
// environment variables so the same binary behaves correctly in dev, CI and
// containers without a rebuild. Core Service is gRPC-only (internal, not
// reachable directly by end users) — see GatewayConfig for the public HTTP entrypoint.
type CoreConfig struct {
	GRPCAddr    string // address the gRPC server listens on
	DatabaseURL string // postgres connection string (DSN)
	Debug       bool   // enables human-readable (non-JSON) logging
	RedisAddr   string // redis address for the link cache (host:port)
	CacheTTL    time.Duration
}

func LoadCore() CoreConfig {
	return CoreConfig{
		GRPCAddr:    getEnv("GRPC_ADDR", ":9090"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/urlshortener?sslmode=disable"),
		Debug:       getEnv("DEBUG", "false") == "true",
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		CacheTTL:    getEnvDuration("CACHE_TTL_SECONDS", 300) * time.Second,
	}
}

// GatewayConfig holds runtime settings for the API Gateway — the single
// public HTTP entrypoint, which reaches Core Service over gRPC.
type GatewayConfig struct {
	HTTPAddr     string // address the public HTTP server listens on
	CoreGRPCAddr string // address of the Core Service gRPC server
	BaseURL      string // public base URL used to build short-link URLs in API responses
	Debug        bool   // enables human-readable (non-JSON) logging
}

func LoadGateway() GatewayConfig {
	return GatewayConfig{
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		CoreGRPCAddr: getEnv("CORE_GRPC_ADDR", "localhost:9090"),
		BaseURL:      getEnv("BASE_URL", "http://localhost:8080"),
		Debug:        getEnv("DEBUG", "false") == "true",
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
