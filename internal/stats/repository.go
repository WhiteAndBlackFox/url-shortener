package stats

import "context"

// Repository is the persistence contract the Service depends on.
// Concrete implementations (Postgres, etc.) live outside this package.
type Repository interface {
	InsertBatch(ctx context.Context, events []ClickEvent) error
	GetStats(ctx context.Context, code string) (*Stats, error)
}
