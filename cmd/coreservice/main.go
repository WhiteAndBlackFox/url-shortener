package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"URLShortener/internal/cache"
	"URLShortener/internal/coreservice/repository/postgres"
	httpapi "URLShortener/internal/coreservice/transport/http"
	"URLShortener/internal/link"
	"URLShortener/internal/platform/config"
	"URLShortener/internal/platform/logger"
	platformpg "URLShortener/internal/platform/postgres"
	platformredis "URLShortener/internal/platform/redis"

	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	log, err := logger.New(cfg.Debug)
	if err != nil {
		panic(err) // logger itself failed to construct; nothing to log to yet
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := platformpg.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("connect to postgres", zap.Error(err))
	}

	redisClient, err := platformredis.NewClient(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatal("connect to redis", zap.Error(err))
	}

	repo := postgres.New(db)
	cachedRepo := cache.NewLinkRepository(repo, redisClient, cfg.CacheTTL, log)
	service := link.NewService(cachedRepo)
	handler := httpapi.NewHandler(service, cfg.BaseURL, log)
	router := httpapi.NewRouter(handler, log)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	go func() {
		log.Info("starting core service", zap.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("http server", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}
}
