package grpcmiddleware

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Recovery recovers from panics in RPC handlers, logs them, and returns
// codes.Internal instead of letting the process crash.
func Recovery(log *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered",
					zap.Any("panic", rec),
					zap.String("method", info.FullMethod),
				)
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// Logging logs method, gRPC status code and latency for every call, and
// logs the underlying error for non-domain (Internal) failures — handlers
// return a generic message to the caller, the real cause is logged here.
func Logging(log *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		resp, err = handler(ctx, req)

		code := status.Code(err)
		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.String("code", code.String()),
			zap.Duration("latency", time.Since(start)),
		}
		if code == codes.Internal {
			log.Error("rpc failed", append(fields, zap.Error(err))...)
		} else {
			log.Info("rpc", fields...)
		}
		return resp, err
	}
}
