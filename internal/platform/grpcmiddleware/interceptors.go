package grpcmiddleware

import (
	"context"
	"time"

	"URLShortener/internal/platform/requestid"

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

// RequestID reads the x-request-id metadata Gateway attaches to every
// outgoing call (see requestid.OutgoingContext) and puts it on the context
// so Logging — and any handler — can read it back with requestid.FromContext.
// If a call arrives without one (e.g. a direct grpcurl call during manual
// testing), a fresh ID is generated so logs are still correlatable.
func RequestID() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := requestid.FromIncomingGRPC(ctx)
		if id == "" {
			id = requestid.New()
		}
		return handler(requestid.NewContext(ctx, id), req)
	}
}

// Logging logs method, gRPC status code, latency and the request ID for
// every call, and logs the underlying error for non-domain (Internal)
// failures — handlers return a generic message to the caller, the real
// cause is logged here.
func Logging(log *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		resp, err = handler(ctx, req)

		code := status.Code(err)
		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.String("code", code.String()),
			zap.String("request_id", requestid.FromContext(ctx)),
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
