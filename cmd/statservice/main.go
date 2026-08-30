package main

import (
	"context"
	"net"
	"os/signal"
	"syscall"
	"time"

	"URLShortener/internal/platform/config"
	"URLShortener/internal/platform/healthmonitor"
	"URLShortener/internal/platform/logger"
	platformpg "URLShortener/internal/platform/postgres"
	"URLShortener/internal/platform/rabbitmq"
	"URLShortener/internal/platform/shutdown"
	"URLShortener/internal/stats"
	statsrepo "URLShortener/internal/statservice/repository/postgres"
	statsgrpc "URLShortener/internal/statservice/transport/grpc"
	"URLShortener/internal/statservice/worker"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// consumerTag identifies this consumer to RabbitMQ so shutdown can cancel
// exactly this subscription (Channel.Cancel) rather than tearing down the
// whole connection before in-flight work is settled — see
// internal/platform/rabbitmq.RunResilientConsumer.
const consumerTag = "statservice-worker-pool"

func main() {
	cfg := config.LoadStat()

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

	// consumerCtx (not ctx directly) so shutdown can cancel just the RabbitMQ
	// consumer first, and separately wait for the worker pool to drain,
	// before touching the gRPC server.
	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	deliveries := make(chan amqp.Delivery)
	go rabbitmq.RunResilientConsumer(consumerCtx, cfg.RabbitMQURL, cfg.ClickQueue, consumerTag, log, deliveries)

	repo := statsrepo.New(db)
	service := stats.NewService(repo)

	pool := worker.NewPool(cfg.WorkerCount, cfg.BatchSize, cfg.FlushInterval, service, log)
	workersDone := pool.Start(deliveries)

	statsServer := statsgrpc.NewStatsServer(service, log)
	grpcServer, healthServer := statsgrpc.NewServer(statsServer, log)

	// Keeps the gRPC health status truthful after startup, not just at it.
	// RabbitMQ reachability is deliberately excluded: RunResilientConsumer
	// already self-heals from broker outages in the background, and
	// flapping the health status over ordinary reconnect churn would be
	// counterproductive (an orchestrator restart mid-backoff just resets
	// the backoff timer, it doesn't fix the underlying outage). Postgres,
	// on the other hand, is a hard dependency Stat Service cannot serve
	// reads without.
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
		log.Info("starting stat service",
			zap.String("addr", cfg.GRPCAddr),
			zap.Int("workers", cfg.WorkerCount),
			zap.Int("batch_size", cfg.BatchSize),
		)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal("grpc server", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	// Cancel the RabbitMQ consumer (not just close the channel/connection):
	// this stops new deliveries while letting in-flight ones drain into the
	// worker pool and get acked normally, so a worker's final flush never
	// has to Ack/Nack on an already-closed channel.
	cancelConsumer()
	if !shutdown.WaitTimeout(10*time.Second, workersDone.Wait) {
		log.Warn("worker pool did not finish draining in time, shutting down anyway")
	}

	if !shutdown.WaitTimeout(5*time.Second, grpcServer.GracefulStop) {
		log.Warn("grpc graceful stop timed out, forcing")
		grpcServer.Stop()
	}
}
