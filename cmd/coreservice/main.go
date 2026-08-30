package main

import (
	"context"
	"net"
	"os/signal"
	"syscall"
	"time"

	"URLShortener/internal/cache"
	"URLShortener/internal/coreservice/repository/postgres"
	coregrpc "URLShortener/internal/coreservice/transport/grpc"
	"URLShortener/internal/link"
	"URLShortener/internal/platform/config"
	"URLShortener/internal/platform/healthmonitor"
	"URLShortener/internal/platform/logger"
	platformpg "URLShortener/internal/platform/postgres"
	platformredis "URLShortener/internal/platform/redis"
	"URLShortener/internal/platform/shutdown"

	"go.uber.org/zap"
)

func main() {
	cfg := config.LoadCore()

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
	linkServer := coregrpc.NewLinkServer(service, log)
	grpcServer, healthServer := coregrpc.NewServer(linkServer, log)

	// Keeps the gRPC health status truthful after startup, not just at it —
	// Postgres is the one dependency Core Service cannot function without;
	// Redis is deliberately excluded since the cache already degrades
	// gracefully on its own (see internal/cache/link.go) and losing it isn't
	// actually a reason to stop routing traffic here.
	go healthmonitor.Run(ctx, healthServer, 10*time.Second, log, map[string]healthmonitor.Checker{
		"postgres": func(ctx context.Context) error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		},
	})

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	go func() {
		log.Info("starting core service", zap.String("addr", cfg.GRPCAddr))
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal("grpc server", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	if !shutdown.WaitTimeout(5*time.Second, grpcServer.GracefulStop) {
		log.Warn("grpc graceful stop timed out, forcing")
		grpcServer.Stop()
	}
}
