package grpc

import (
	"context"
	"errors"

	linkpb "URLShortener/api/proto/linkpb"
	"URLShortener/internal/link"

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
}

func NewLinkServer(service *link.Service) *LinkServer {
	return &LinkServer{service: service}
}

func (s *LinkServer) CreateLink(ctx context.Context, req *linkpb.CreateLinkRequest) (*linkpb.Link, error) {
	l, err := s.service.CreateLink(ctx, req.GetUrl())
	if err != nil {
		return nil, toStatusError(err)
	}
	return toProto(l), nil
}

func (s *LinkServer) GetLink(ctx context.Context, req *linkpb.GetLinkRequest) (*linkpb.Link, error) {
	l, err := s.service.ResolveCode(ctx, req.GetCode())
	if err != nil {
		return nil, toStatusError(err)
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

func toStatusError(err error) error {
	switch {
	case errors.Is(err, link.ErrInvalidURL):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, link.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
