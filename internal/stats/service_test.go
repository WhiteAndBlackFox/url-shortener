package stats_test

import (
	"context"
	"testing"
	"time"

	"URLShortener/internal/stats"

	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	inserted []stats.ClickEvent
	byCode   map[string]*stats.Stats
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byCode: make(map[string]*stats.Stats)}
}

func (r *fakeRepo) InsertBatch(_ context.Context, events []stats.ClickEvent) error {
	r.inserted = append(r.inserted, events...)
	return nil
}

func (r *fakeRepo) GetStats(_ context.Context, code string) (*stats.Stats, error) {
	if s, ok := r.byCode[code]; ok {
		return s, nil
	}
	return &stats.Stats{Code: code, TotalClicks: 0}, nil
}

func TestService_RecordClicks(t *testing.T) {
	repo := newFakeRepo()
	svc := stats.NewService(repo)

	events := []stats.ClickEvent{
		{Code: "abc1234", OccurredAt: time.Now()},
		{Code: "abc1234", OccurredAt: time.Now()},
	}
	require.NoError(t, svc.RecordClicks(context.Background(), events))
	require.Len(t, repo.inserted, 2)
}

func TestService_RecordClicks_Empty(t *testing.T) {
	repo := newFakeRepo()
	svc := stats.NewService(repo)

	require.NoError(t, svc.RecordClicks(context.Background(), nil))
	require.Empty(t, repo.inserted)
}

func TestService_GetStats_UnknownCodeIsZeroNotError(t *testing.T) {
	repo := newFakeRepo()
	svc := stats.NewService(repo)

	got, err := svc.GetStats(context.Background(), "doesnotexist")
	require.NoError(t, err)
	require.Equal(t, int64(0), got.TotalClicks)
}
