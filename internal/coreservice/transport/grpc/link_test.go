package grpc_test

import (
	"context"
	"testing"

	linkpb "URLShortener/api/proto/linkpb"
	coregrpc "URLShortener/internal/coreservice/transport/grpc"
	"URLShortener/internal/link"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeRepo is the same style of in-memory link.Repository used in
// internal/link/service_test.go, kept local here since that one is unexported.
type fakeRepo struct {
	links map[string]*link.Link
}

func newFakeRepo() *fakeRepo { return &fakeRepo{links: make(map[string]*link.Link)} }

func (r *fakeRepo) Create(_ context.Context, l *link.Link) error {
	r.links[l.Code] = l
	return nil
}

func (r *fakeRepo) GetByCode(_ context.Context, code string) (*link.Link, error) {
	l, ok := r.links[code]
	if !ok {
		return nil, link.ErrNotFound
	}
	return l, nil
}

func newTestServer() *coregrpc.LinkServer {
	service := link.NewService(newFakeRepo())
	return coregrpc.NewLinkServer(service)
}

func TestLinkServer_CreateLink(t *testing.T) {
	srv := newTestServer()

	resp, err := srv.CreateLink(context.Background(), &linkpb.CreateLinkRequest{Url: "https://example.com"})
	require.NoError(t, err)
	require.NotEmpty(t, resp.GetCode())
	require.Equal(t, "https://example.com", resp.GetLongUrl())
	require.NotNil(t, resp.GetCreatedAt())
}

func TestLinkServer_CreateLink_InvalidURL(t *testing.T) {
	srv := newTestServer()

	_, err := srv.CreateLink(context.Background(), &linkpb.CreateLinkRequest{Url: "not-a-url"})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestLinkServer_GetLink_NotFound(t *testing.T) {
	srv := newTestServer()

	_, err := srv.GetLink(context.Background(), &linkpb.GetLinkRequest{Code: "doesnotexist"})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestLinkServer_CreateThenGetLink(t *testing.T) {
	srv := newTestServer()

	created, err := srv.CreateLink(context.Background(), &linkpb.CreateLinkRequest{Url: "https://example.com/x"})
	require.NoError(t, err)

	got, err := srv.GetLink(context.Background(), &linkpb.GetLinkRequest{Code: created.GetCode()})
	require.NoError(t, err)
	require.Equal(t, created.GetCode(), got.GetCode())
	require.Equal(t, "https://example.com/x", got.GetLongUrl())
}
