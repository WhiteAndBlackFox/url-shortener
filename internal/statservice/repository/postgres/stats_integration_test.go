package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	platformpg "URLShortener/internal/platform/postgres"
	"URLShortener/internal/statservice/repository/postgres"
	"URLShortener/internal/stats"

	"github.com/stretchr/testify/require"
)

func TestRepo_InsertBatchAndGetStats(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := postgres.New(db)
	code := "stsint01"
	t.Cleanup(func() { db.Exec("DELETE FROM clicks WHERE code = ?", code) })

	before, err := repo.GetStats(ctx, code)
	require.NoError(t, err)
	require.Equal(t, int64(0), before.TotalClicks, "no rows yet: must report zero, not error")

	now := time.Now().UTC().Truncate(time.Second)
	events := []stats.ClickEvent{
		{Code: code, OccurredAt: now, IP: "1.1.1.1"},
		{Code: code, OccurredAt: now.Add(time.Minute), IP: "2.2.2.2"},
		{Code: code, OccurredAt: now.Add(2 * time.Minute), IP: "3.3.3.3"},
	}
	require.NoError(t, repo.InsertBatch(ctx, events))

	after, err := repo.GetStats(ctx, code)
	require.NoError(t, err)
	require.Equal(t, int64(3), after.TotalClicks)
	require.WithinDuration(t, now.Add(2*time.Minute), after.LastClickedAt, time.Second)
}
