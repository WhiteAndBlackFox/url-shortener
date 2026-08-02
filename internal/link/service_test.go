package link_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"URLShortener/internal/link"

	"github.com/stretchr/testify/require"
)

// fakeRepo is an in-memory link.Repository used to test link.Service in isolation.
type fakeRepo struct {
	mu             sync.Mutex
	links          map[string]*link.Link
	forceConflicts int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{links: make(map[string]*link.Link)}
}

func (r *fakeRepo) Create(_ context.Context, l *link.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.forceConflicts > 0 {
		r.forceConflicts--
		return link.ErrCodeConflict
	}
	if _, exists := r.links[l.Code]; exists {
		return link.ErrCodeConflict
	}
	r.links[l.Code] = l
	return nil
}

func (r *fakeRepo) GetByCode(_ context.Context, code string) (*link.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	l, ok := r.links[code]
	if !ok {
		return nil, link.ErrNotFound
	}
	return l, nil
}

func TestService_CreateLink_Valid(t *testing.T) {
	svc := link.NewService(newFakeRepo())

	l, err := svc.CreateLink(context.Background(), "https://example.com/path")
	require.NoError(t, err)
	require.Len(t, l.Code, 7)
	require.Equal(t, "https://example.com/path", l.LongURL)
}

func TestService_CreateLink_InvalidURL(t *testing.T) {
	svc := link.NewService(newFakeRepo())

	cases := []string{"", "not-a-url", "ftp://example.com", "https://"}
	for _, raw := range cases {
		_, err := svc.CreateLink(context.Background(), raw)
		require.ErrorIs(t, err, link.ErrInvalidURL, "input: %q", raw)
	}
}

func TestService_CreateLink_RetriesOnCodeConflict(t *testing.T) {
	repo := newFakeRepo()
	repo.forceConflicts = 2 // first two generated codes will "collide"
	svc := link.NewService(repo)

	l, err := svc.CreateLink(context.Background(), "https://example.com")
	require.NoError(t, err)
	require.NotEmpty(t, l.Code)
}

func TestService_CreateLink_GivesUpAfterTooManyConflicts(t *testing.T) {
	repo := newFakeRepo()
	repo.forceConflicts = 100 // always conflict, exceeds internal retry budget
	svc := link.NewService(repo)

	_, err := svc.CreateLink(context.Background(), "https://example.com")
	require.Error(t, err)
	require.False(t, errors.Is(err, link.ErrInvalidURL))
}

func TestService_ResolveCode(t *testing.T) {
	repo := newFakeRepo()
	svc := link.NewService(repo)

	created, err := svc.CreateLink(context.Background(), "https://example.com")
	require.NoError(t, err)

	found, err := svc.ResolveCode(context.Background(), created.Code)
	require.NoError(t, err)
	require.Equal(t, created.LongURL, found.LongURL)

	_, err = svc.ResolveCode(context.Background(), "doesnotexist")
	require.ErrorIs(t, err, link.ErrNotFound)
}
