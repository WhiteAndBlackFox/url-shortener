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
	"URLShortener/internal/platform/rabbitmq"

	"go.uber.org/zap"
)

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

	mqConn, mqCh, err := rabbitmq.Dial(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal("dial rabbitmq", zap.Error(err))
	}
	defer func() { _ = mqCh.Close() }()
	defer func() { _ = mqConn.Close() }()

	if _, err := rabbitmq.DeclareQueue(mqCh, cfg.ClickQueue); err != nil {
		log.Fatal("declare click queue", zap.Error(err))
	}
	clickPublisher := publisher.New(mqCh, cfg.ClickQueue)

	linkClient := linkpb.NewLinkServiceClient(coreConn)
	statsClient := statspb.NewStatsServiceClient(statConn)
	handler := httpapi.NewHandler(linkClient, cfg.BaseURL, log, clickPublisher)
	statsHandler := httpapi.NewStatsHandler(statsClient, log)
	router := httpapi.NewRouter(handler, statsHandler, log)

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
}
