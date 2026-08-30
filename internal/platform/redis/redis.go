package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient creates and validates a Redis client.
func NewClient(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return client, nil
}
