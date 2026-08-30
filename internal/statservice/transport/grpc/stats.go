package grpc

import (
	"context"
	"errors"

	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/stats"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StatsServer implements statspb.StatsServiceServer on top of stats.Service.
type StatsServer struct {
	statspb.UnimplementedStatsServiceServer
	service *stats.Service
	log     *zap.Logger
}

func NewStatsServer(service *stats.Service, log *zap.Logger) *StatsServer {
	return &StatsServer{service: service, log: log}
}

func (s *StatsServer) GetStats(ctx context.Context, req *statspb.GetStatsRequest) (*statspb.StatsResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}

	st, err := s.service.GetStats(ctx, req.GetCode())
	if err != nil {
		return nil, s.toStatusError(err)
	}

	return &statspb.StatsResponse{
		Code:          st.Code,
		TotalClicks:   st.TotalClicks,
		LastClickedAt: timestamppb.New(st.LastClickedAt),
	}, nil
}

// toStatusError mirrors coreservice/transport/grpc.LinkServer.toStatusError:
// client cancellation/deadline expiry get their own gRPC codes instead of a
// generic Internal (grpcmiddleware.Logging only logs Error-level for
// codes.Internal, so misclassifying a routine cancellation would read as a
// real failure), and the real cause is logged here before being discarded
// from the client-facing message.
func (s *StatsServer) toStatusError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	default:
		s.log.Error("internal error", zap.Error(err))
		return status.Error(codes.Internal, "internal server error")
	}
}
