package cache_test

import (
	"context"
	"testing"
	"time"

	"URLShortener/internal/cache"
	"URLShortener/internal/link"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// countingRepo wraps an in-memory map and counts GetByCode calls, so tests
// can assert whether the underlying repository was actually hit.
type countingRepo struct {
	links     map[string]*link.Link
	getCalls  int
	createErr error
}

func newCountingRepo() *countingRepo {
	return &countingRepo{links: make(map[string]*link.Link)}
}

func (r *countingRepo) Create(_ context.Context, l *link.Link) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.links[l.Code] = l
	return nil
}

func (r *countingRepo) GetByCode(_ context.Context, code string) (*link.Link, error) {
	r.getCalls++
	l, ok := r.links[code]
	if !ok {
		return nil, link.ErrNotFound
	}
	return l, nil
}

func newTestRepo(t *testing.T, next *countingRepo) *cache.LinkRepository {
	t.Helper()

	mr := miniredis.RunT(t) // in-memory redis, torn down automatically at test end
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return cache.NewLinkRepository(next, client, time.Minute, zap.NewNop())
}

func TestLinkRepository_GetByCode_MissThenHit(t *testing.T) {
	next := newCountingRepo()
	next.links["abc1234"] = &link.Link{Code: "abc1234", LongURL: "https://example.com"}
	repo := newTestRepo(t, next)
	ctx := context.Background()

	// First call: cache miss, falls through to next and populates the cache.
	l, err := repo.GetByCode(ctx, "abc1234")
	require.NoError(t, err)
	require.Equal(t, "https://example.com", l.LongURL)
	require.Equal(t, 1, next.getCalls)

	// Second call: served from cache, next is not called again.
	l, err = repo.GetByCode(ctx, "abc1234")
	require.NoError(t, err)
	require.Equal(t, "https://example.com", l.LongURL)
	require.Equal(t, 1, next.getCalls, "expected cache hit, underlying repo should not be called again")
}

func TestLinkRepository_GetByCode_NotFoundIsNotCached(t *testing.T) {
	next := newCountingRepo()
	repo := newTestRepo(t, next)
	ctx := context.Background()

	_, err := repo.GetByCode(ctx, "doesnotexist")
	require.ErrorIs(t, err, link.ErrNotFound)
	require.Equal(t, 1, next.getCalls)

	_, err = repo.GetByCode(ctx, "doesnotexist")
	require.ErrorIs(t, err, link.ErrNotFound)
	require.Equal(t, 2, next.getCalls, "not-found results must not be cached")
}

func TestLinkRepository_Create_DelegatesToNext(t *testing.T) {
	next := newCountingRepo()
	repo := newTestRepo(t, next)
	ctx := context.Background()

	l := &link.Link{Code: "newcode1", LongURL: "https://example.com/new"}
	require.NoError(t, repo.Create(ctx, l))
	require.Contains(t, next.links, "newcode1")
}
