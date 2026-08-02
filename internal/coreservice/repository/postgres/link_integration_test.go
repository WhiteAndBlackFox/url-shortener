package postgres_test

import (
	"context"
	"os"
	"testing"

	"URLShortener/internal/coreservice/repository/postgres"
	"URLShortener/internal/link"
	platformpg "URLShortener/internal/platform/postgres"

	"github.com/stretchr/testify/require"
)

func TestRepo_CreateAndGetByCode(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := postgres.New(db)

	l := &link.Link{Code: "itc0de0001", LongURL: "https://example.com/integration-test"}
	t.Cleanup(func() { db.Exec("DELETE FROM links WHERE code = ?", l.Code) })

	require.NoError(t, repo.Create(ctx, l))
	require.False(t, l.CreatedAt.IsZero())

	found, err := repo.GetByCode(ctx, l.Code)
	require.NoError(t, err)
	require.Equal(t, l.Code, found.Code)
	require.Equal(t, l.LongURL, found.LongURL)

	_, err = repo.GetByCode(ctx, "doesnotexist")
	require.ErrorIs(t, err, link.ErrNotFound)
}

func TestRepo_Create_DuplicateCodeConflict(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := postgres.New(db)

	code := "itc0de0002"
	t.Cleanup(func() { db.Exec("DELETE FROM links WHERE code = ?", code) })

	require.NoError(t, repo.Create(ctx, &link.Link{Code: code, LongURL: "https://example.com/one"}))

	err = repo.Create(ctx, &link.Link{Code: code, LongURL: "https://example.com/two"})
	require.ErrorIs(t, err, link.ErrCodeConflict)
}
