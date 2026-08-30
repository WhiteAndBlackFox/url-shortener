package grpc

import (
	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/platform/grpcmiddleware"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// NewServer wires interceptors and registers StatsServer on a new gRPC
// server for Stat Service.
func NewServer(statsServer *StatsServer, log *zap.Logger) *grpc.Server {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcmiddleware.Recovery(log), grpcmiddleware.Logging(log)),
	)
	statspb.RegisterStatsServiceServer(srv, statsServer)
	return srv
}
