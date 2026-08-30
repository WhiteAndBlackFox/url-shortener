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
// public HTTP entrypoint, which reaches Core Service and Stat Service over
// gRPC and publishes click events to RabbitMQ.
type GatewayConfig struct {
	HTTPAddr     string // address the public HTTP server listens on
	CoreGRPCAddr string // address of the Core Service gRPC server
	StatGRPCAddr string // address of the Stat Service gRPC server
	BaseURL      string // public base URL used to build short-link URLs in API responses
	Debug        bool   // enables human-readable (non-JSON) logging
	RabbitMQURL  string
	ClickQueue   string // RabbitMQ queue click events are published to
}

func LoadGateway() GatewayConfig {
	return GatewayConfig{
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		CoreGRPCAddr: getEnv("CORE_GRPC_ADDR", "localhost:9090"),
		StatGRPCAddr: getEnv("STAT_GRPC_ADDR", "localhost:9091"),
		BaseURL:      getEnv("BASE_URL", "http://localhost:8080"),
		Debug:        getEnv("DEBUG", "false") == "true",
		RabbitMQURL:  getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		ClickQueue:   getEnv("CLICK_QUEUE", "link.clicks"),
	}
}

// StatConfig holds runtime settings for Stat Service — internal, gRPC-only,
// consumes click events from RabbitMQ and batch-writes them to Postgres.
type StatConfig struct {
	GRPCAddr      string
	DatabaseURL   string
	Debug         bool
	RabbitMQURL   string
	ClickQueue    string
	WorkerCount   int
	BatchSize     int
	FlushInterval time.Duration
}

func LoadStat() StatConfig {
	return StatConfig{
		GRPCAddr:      getEnv("GRPC_ADDR", ":9091"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/urlshortener?sslmode=disable"),
		Debug:         getEnv("DEBUG", "false") == "true",
		RabbitMQURL:   getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		ClickQueue:    getEnv("CLICK_QUEUE", "link.clicks"),
		WorkerCount:   getEnvInt("WORKER_COUNT", 4),
		BatchSize:     getEnvInt("BATCH_SIZE", 50),
		FlushInterval: getEnvDuration("FLUSH_INTERVAL_MS", 500) * time.Millisecond,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvDuration(key string, fallbackUnits int) time.Duration {
	return time.Duration(getEnvInt(key, fallbackUnits))
}
