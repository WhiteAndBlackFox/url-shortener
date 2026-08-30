package grpc

import (
	"context"
	"errors"

	linkpb "URLShortener/api/proto/linkpb"
	"URLShortener/internal/link"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// LinkServer implements linkpb.LinkServiceServer on top of link.Service.
// It is the gRPC equivalent of the old httpapi.Handler from phase 1-3: same
// domain calls, different transport, same job of translating domain errors
// into transport-appropriate codes (gRPC status codes instead of HTTP ones).
type LinkServer struct {
	linkpb.UnimplementedLinkServiceServer
	service *link.Service
	log     *zap.Logger
}

func NewLinkServer(service *link.Service, log *zap.Logger) *LinkServer {
	return &LinkServer{service: service, log: log}
}

func (s *LinkServer) CreateLink(ctx context.Context, req *linkpb.CreateLinkRequest) (*linkpb.Link, error) {
	l, err := s.service.CreateLink(ctx, req.GetUrl())
	if err != nil {
		return nil, s.toStatusError(err)
	}
	return toProto(l), nil
}

func (s *LinkServer) GetLink(ctx context.Context, req *linkpb.GetLinkRequest) (*linkpb.Link, error) {
	l, err := s.service.ResolveCode(ctx, req.GetCode())
	if err != nil {
		return nil, s.toStatusError(err)
	}
	return toProto(l), nil
}

func toProto(l *link.Link) *linkpb.Link {
	return &linkpb.Link{
		Code:      l.Code,
		LongUrl:   l.LongURL,
		CreatedAt: timestamppb.New(l.CreatedAt),
	}
}

// toStatusError maps a domain/infra error to a gRPC status. Client
// cancellation and deadline expiry get their own dedicated gRPC codes
// (rather than falling into the generic Internal bucket) since they're
// normal, expected outcomes — a closed browser tab, not a server bug — and
// grpcmiddleware.Logging only logs at Error level for codes.Internal, so
// misclassifying them would falsely read as real failures in the logs.
// Everything else genuinely is unexpected, so — unlike the client-facing
// generic message — the real cause is logged here before it's discarded,
// instead of being lost entirely.
func (s *LinkServer) toStatusError(err error) error {
	switch {
	case errors.Is(err, link.ErrInvalidURL):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, link.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	default:
		s.log.Error("internal error", zap.Error(err))
		return status.Error(codes.Internal, "internal server error")
	}
}
