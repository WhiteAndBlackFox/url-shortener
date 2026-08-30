package grpc

import (
	linkpb "URLShortener/api/proto/linkpb"
	"URLShortener/internal/platform/grpcmiddleware"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// NewServer wires interceptors and registers LinkServer on a new gRPC server
// for the Core Service.
func NewServer(linkServer *LinkServer, log *zap.Logger) *grpc.Server {
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpcmiddleware.Recovery(log), grpcmiddleware.Logging(log)),
	)
	linkpb.RegisterLinkServiceServer(srv, linkServer)
	return srv
}
