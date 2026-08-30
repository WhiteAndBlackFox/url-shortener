package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	platformpg "URLShortener/internal/platform/postgres"
	"URLShortener/internal/stats"
	"URLShortener/internal/statservice/repository/postgres"

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
		{EventID: "stsint01-evt-1", Code: code, OccurredAt: now, IP: "1.1.1.1"},
		{EventID: "stsint01-evt-2", Code: code, OccurredAt: now.Add(time.Minute), IP: "2.2.2.2"},
		{EventID: "stsint01-evt-3", Code: code, OccurredAt: now.Add(2 * time.Minute), IP: "3.3.3.3"},
	}
	require.NoError(t, repo.InsertBatch(ctx, events))

	after, err := repo.GetStats(ctx, code)
	require.NoError(t, err)
	require.Equal(t, int64(3), after.TotalClicks)
	require.WithinDuration(t, now.Add(2*time.Minute), after.LastClickedAt, time.Second)
}

// TestRepo_InsertBatch_DuplicateEventIDIsIdempotent proves redelivery safety:
// RabbitMQ's at-least-once delivery means the same event can be handed to
// InsertBatch more than once (e.g. a Nack(requeue=true) after a transient
// write failure, or a flush timeout that fires right as the write actually
// committed) — a duplicate event_id must not inflate the click count.
func TestRepo_InsertBatch_DuplicateEventIDIsIdempotent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := postgres.New(db)
	code := "stsint02"
	t.Cleanup(func() { db.Exec("DELETE FROM clicks WHERE code = ?", code) })

	event := stats.ClickEvent{EventID: "stsint02-evt-dup", Code: code, OccurredAt: time.Now().UTC()}

	require.NoError(t, repo.InsertBatch(ctx, []stats.ClickEvent{event}))
	require.NoError(t, repo.InsertBatch(ctx, []stats.ClickEvent{event})) // redelivery of the same event

	after, err := repo.GetStats(ctx, code)
	require.NoError(t, err)
	require.Equal(t, int64(1), after.TotalClicks, "redelivering the same event_id must not double-count")
}
