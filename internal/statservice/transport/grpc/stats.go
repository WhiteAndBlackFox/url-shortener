package grpc

import (
	"context"

	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/stats"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StatsServer implements statspb.StatsServiceServer on top of stats.Service.
type StatsServer struct {
	statspb.UnimplementedStatsServiceServer
	service *stats.Service
}

func NewStatsServer(service *stats.Service) *StatsServer {
	return &StatsServer{service: service}
}

func (s *StatsServer) GetStats(ctx context.Context, req *statspb.GetStatsRequest) (*statspb.StatsResponse, error) {
	if req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}

	st, err := s.service.GetStats(ctx, req.GetCode())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal server error")
	}

	return &statspb.StatsResponse{
		Code:          st.Code,
		TotalClicks:   st.TotalClicks,
		LastClickedAt: timestamppb.New(st.LastClickedAt),
	}, nil
}
