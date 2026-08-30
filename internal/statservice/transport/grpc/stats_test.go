package grpc_test

import (
	"context"
	"testing"
	"time"

	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/stats"
	statsgrpc "URLShortener/internal/statservice/transport/grpc"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeRepo struct {
	byCode map[string]*stats.Stats
}

func (r *fakeRepo) InsertBatch(_ context.Context, _ []stats.ClickEvent) error { return nil }

func (r *fakeRepo) GetStats(_ context.Context, code string) (*stats.Stats, error) {
	if s, ok := r.byCode[code]; ok {
		return s, nil
	}
	return &stats.Stats{Code: code}, nil
}

func TestStatsServer_GetStats(t *testing.T) {
	repo := &fakeRepo{byCode: map[string]*stats.Stats{
		"abc1234": {Code: "abc1234", TotalClicks: 42, LastClickedAt: time.Now()},
	}}
	srv := statsgrpc.NewStatsServer(stats.NewService(repo), zap.NewNop())

	resp, err := srv.GetStats(context.Background(), &statspb.GetStatsRequest{Code: "abc1234"})
	require.NoError(t, err)
	require.Equal(t, int64(42), resp.GetTotalClicks())
}

func TestStatsServer_GetStats_UnknownCodeReturnsZero(t *testing.T) {
	repo := &fakeRepo{byCode: map[string]*stats.Stats{}}
	srv := statsgrpc.NewStatsServer(stats.NewService(repo), zap.NewNop())

	resp, err := srv.GetStats(context.Background(), &statspb.GetStatsRequest{Code: "doesnotexist"})
	require.NoError(t, err)
	require.Equal(t, int64(0), resp.GetTotalClicks())
}

func TestStatsServer_GetStats_EmptyCodeIsInvalidArgument(t *testing.T) {
	srv := statsgrpc.NewStatsServer(stats.NewService(&fakeRepo{byCode: map[string]*stats.Stats{}}), zap.NewNop())

	_, err := srv.GetStats(context.Background(), &statspb.GetStatsRequest{Code: ""})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestStatsServer_GetStats_ContextCanceledIsNotInternal(t *testing.T) {
	repo := &cancelingRepo{}
	srv := statsgrpc.NewStatsServer(stats.NewService(repo), zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := srv.GetStats(ctx, &statspb.GetStatsRequest{Code: "abc1234"})
	require.Error(t, err)
	require.Equal(t, codes.Canceled, status.Code(err), "a client cancellation is not a server bug — must not surface as codes.Internal")
}

// cancelingRepo simulates a repository call that observes ctx cancellation
// (as a real Postgres/gorm call would) instead of an internal server bug.
type cancelingRepo struct{}

func (cancelingRepo) InsertBatch(ctx context.Context, _ []stats.ClickEvent) error { return ctx.Err() }
func (cancelingRepo) GetStats(ctx context.Context, _ string) (*stats.Stats, error) {
	return nil, ctx.Err()
}
