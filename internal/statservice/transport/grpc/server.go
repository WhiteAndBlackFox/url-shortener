package grpc

import (
	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/platform/grpcmiddleware"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// NewServer wires interceptors and registers StatsServer on a new gRPC
// server for Stat Service. It also registers the standard gRPC
// health-checking protocol (grpc_health_v1), which is what docker-compose's
// healthcheck probes via grpc-health-probe — see
// deployments/docker/statservice.Dockerfile.
//
// The returned *health.Server is handed back to the caller (not just kept
// internal) so main.go can feed it into healthmonitor.Run — SetServingStatus
// here only covers "did we start up successfully", not "are we still healthy
// now"; the periodic monitor is what keeps it truthful afterward.
func NewServer(statsServer *StatsServer, log *zap.Logger) (*grpc.Server, *health.Server) {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcmiddleware.Recovery(log), grpcmiddleware.RequestID(), grpcmiddleware.Logging(log)),
	)
	statspb.RegisterStatsServiceServer(srv, statsServer)

	// By the time main() calls NewServer, the Postgres connection and the
	// RabbitMQ consumer it depends on have already been validated (main.go
	// exits via log.Fatal otherwise), so it's safe to report SERVING immediately.
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, healthServer)

	// Reflection lets grpcurl (and similar tools) call this server without a
	// local copy of stats.proto — convenient for manual debugging. Safe here
	// because Stat Service's gRPC port is never exposed outside the private
	// docker-compose network in this project (published to the host only
	// for that same local debugging).
	reflection.Register(srv)

	return srv, healthServer
}
