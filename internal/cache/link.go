package cache

import (
	"context"
	"encoding/json"
	"time"

	"URLShortener/internal/link"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const keyPrefix = "link:"

// LinkRepository is a cache-aside decorator around a link.Repository: it
// implements the same interface, so link.Service and the HTTP handlers are
// completely unaware it exists (see main.go wiring).
//
// Only GetByCode is cached, since it's the hot path (redirects). Create
// writes through to next only — Postgres stays the single source of truth,
// and the first read after creation populates the cache on a miss.
//
// There is currently no update/delete endpoint (Link is immutable once
// created), so there is nothing to invalidate. If a mutation endpoint is
// added later, it must delete the corresponding cache key here.
type LinkRepository struct {
	next  link.Repository
	redis *redis.Client
	ttl   time.Duration
	log   *zap.Logger
}

func NewLinkRepository(next link.Repository, redisClient *redis.Client, ttl time.Duration, log *zap.Logger) *LinkRepository {
	return &LinkRepository{next: next, redis: redisClient, ttl: ttl, log: log}
}

func (r *LinkRepository) Create(ctx context.Context, l *link.Link) error {
	return r.next.Create(ctx, l)
}

func (r *LinkRepository) GetByCode(ctx context.Context, code string) (*link.Link, error) {
	if l, err := r.getCached(ctx, code); err == nil {
		r.log.Debug("cache hit", zap.String("code", code))
		return l, nil
	}
	r.log.Debug("cache miss", zap.String("code", code))

	l, err := r.next.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	r.setCached(ctx, l)
	return l, nil
}

// getCached returns redis.Nil (wrapped) on a cache miss, and any other error
// (bad connection, corrupt payload) is treated the same way by the caller:
// fall through to the underlying repository rather than fail the request.
func (r *LinkRepository) getCached(ctx context.Context, code string) (*link.Link, error) {
	raw, err := r.redis.Get(ctx, keyPrefix+code).Bytes()
	if err != nil {
		return nil, err // includes redis.Nil on miss
	}

	var l link.Link
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// setCached best-effort populates the cache; a failure here must not fail
// the request, since Postgres already has the authoritative answer.
func (r *LinkRepository) setCached(ctx context.Context, l *link.Link) {
	raw, err := json.Marshal(l)
	if err != nil {
		return
	}
	_ = r.redis.Set(ctx, keyPrefix+l.Code, raw, r.ttl).Err()
}
