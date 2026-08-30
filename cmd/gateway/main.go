package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	linkpb "URLShortener/api/proto/linkpb"
	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/gateway/client"
	"URLShortener/internal/gateway/publisher"
	httpapi "URLShortener/internal/gateway/transport/http"
	"URLShortener/internal/platform/config"
	"URLShortener/internal/platform/logger"

	"go.uber.org/zap"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// @title			URL Shortener API
// @version		1.0
// @description	Public REST API for the URL Shortener project. This is the only public HTTP surface in the system — Core Service and Stat Service are internal, gRPC-only, and reached through this Gateway.
// @BasePath		/
func main() {
	cfg := config.LoadGateway()

	log, err := logger.New(cfg.Debug)
	if err != nil {
		panic(err) // logger itself failed to construct; nothing to log to yet
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	coreConn, err := client.Dial(cfg.CoreGRPCAddr)
	if err != nil {
		log.Fatal("dial core service", zap.Error(err))
	}
	defer func() { _ = coreConn.Close() }()

	statConn, err := client.Dial(cfg.StatGRPCAddr)
	if err != nil {
		log.Fatal("dial stat service", zap.Error(err))
	}
	defer func() { _ = statConn.Close() }()

	// ClickPublisher owns its own RabbitMQ connection end-to-end (dial,
	// declare, reconnect-on-failure) — see internal/gateway/publisher.
	clickPublisher, err := publisher.New(cfg.RabbitMQURL, cfg.ClickQueue)
	if err != nil {
		log.Fatal("connect click publisher", zap.Error(err))
	}

	linkClient := linkpb.NewLinkServiceClient(coreConn)
	statsClient := statspb.NewStatsServiceClient(statConn)
	handler := httpapi.NewHandler(linkClient, cfg.BaseURL, log, clickPublisher)
	statsHandler := httpapi.NewStatsHandler(statsClient, log)
	readinessHandler := httpapi.NewReadinessHandler(
		healthpb.NewHealthClient(coreConn),
		healthpb.NewHealthClient(statConn),
	)
	router := httpapi.NewRouter(handler, statsHandler, readinessHandler, log)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	go func() {
		log.Info("starting gateway",
			zap.String("addr", cfg.HTTPAddr),
			zap.String("core", cfg.CoreGRPCAddr),
			zap.String("stat", cfg.StatGRPCAddr),
		)
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

	// srv.Shutdown only waits for in-flight HTTP handlers, but Redirect's
	// click-publish goroutine is deliberately detached from the request
	// lifecycle (see Handler.publishClickAsync) so it can outlive the
	// response that spawned it — wait for those too before tearing down the
	// publisher, or a click from a request that finished right at shutdown
	// would silently fail to publish.
	handler.WaitPublishers(shutdownCtx)

	if err := clickPublisher.Close(); err != nil {
		log.Error("close click publisher", zap.Error(err))
	}
}
