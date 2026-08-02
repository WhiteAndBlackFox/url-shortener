package link

import "context"

// Repository is the persistence contract the Service depends on.
// Concrete implementations (Postgres, etc.) live outside this package.
type Repository interface {
	Create(ctx context.Context, l *Link) error
	GetByCode(ctx context.Context, code string) (*Link, error)
}
