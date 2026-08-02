package postgres

import (
	"context"
	"fmt"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewDB opens a GORM connection to Postgres and validates it with a ping.
// GORM manages its own connection pool internally via database/sql, which is
// why HTTP handlers can safely share a single *gorm.DB across concurrent requests.
func NewDB(ctx context.Context, dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres: get underlying sql.DB: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return db, nil
}
