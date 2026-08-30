package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	linkpb "URLShortener/api/proto/linkpb"
	"URLShortener/internal/gateway/client"
	httpapi "URLShortener/internal/gateway/transport/http"
	"URLShortener/internal/platform/config"
	"URLShortener/internal/platform/logger"

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

	conn, err := client.Dial(cfg.CoreGRPCAddr)
	if err != nil {
		log.Fatal("dial core service", zap.Error(err))
	}
	defer func() { _ = conn.Close() }()

	linkClient := linkpb.NewLinkServiceClient(conn)
	handler := httpapi.NewHandler(linkClient, cfg.BaseURL, log)
	router := httpapi.NewRouter(handler, log)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router,
	}

	go func() {
		log.Info("starting gateway", zap.String("addr", cfg.HTTPAddr), zap.String("core", cfg.CoreGRPCAddr))
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
