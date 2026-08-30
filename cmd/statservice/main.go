package main

import (
	"context"
	"net"
	"os/signal"
	"syscall"

	"URLShortener/internal/platform/config"
	"URLShortener/internal/platform/logger"
	platformpg "URLShortener/internal/platform/postgres"
	"URLShortener/internal/platform/rabbitmq"
	statsrepo "URLShortener/internal/statservice/repository/postgres"
	statsgrpc "URLShortener/internal/statservice/transport/grpc"
	"URLShortener/internal/statservice/worker"
	"URLShortener/internal/stats"

	"go.uber.org/zap"
)

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

	mqConn, mqCh, err := rabbitmq.Dial(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal("dial rabbitmq", zap.Error(err))
	}

	if _, err := rabbitmq.DeclareQueue(mqCh, cfg.ClickQueue); err != nil {
		log.Fatal("declare click queue", zap.Error(err))
	}

	deliveries, err := mqCh.Consume(
		cfg.ClickQueue,
		"",    // consumer tag: auto-generated
		false, // autoAck: false — the worker pool acks only after a batch is durably written
		false, // exclusive
		false, // noLocal (unused by RabbitMQ)
		false, // noWait
		nil,   // args
	)
	if err != nil {
		log.Fatal("consume click queue", zap.Error(err))
	}

	repo := statsrepo.New(db)
	service := stats.NewService(repo)

	pool := worker.NewPool(cfg.WorkerCount, cfg.BatchSize, cfg.FlushInterval, service, log)
	workersDone := pool.Start(deliveries)

	statsServer := statsgrpc.NewStatsServer(service)
	grpcServer := statsgrpc.NewServer(statsServer, log)

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

	// Closing the channel/connection closes the deliveries channel, which is
	// how the worker pool's goroutines learn to flush their pending batch
	// and exit — see worker.Pool.run.
	_ = mqCh.Close()
	_ = mqConn.Close()
	workersDone.Wait()

	grpcServer.GracefulStop()
}
