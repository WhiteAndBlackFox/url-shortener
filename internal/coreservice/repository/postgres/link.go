package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"URLShortener/internal/link"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// uniqueViolation is the Postgres error code for a unique constraint violation.
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const uniqueViolation = "23505"

// linkRecord is the GORM-mapped row for the "links" table. It is kept private
// and separate from link.Link so the domain package stays free of ORM tags
// and doesn't depend on gorm.
type linkRecord struct {
	Code      string    `gorm:"column:code;primaryKey;size:16"`
	LongURL   string    `gorm:"column:long_url;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (linkRecord) TableName() string {
	return "links"
}

// newRecord converts a domain Link into its GORM row representation.
func newRecord(l *link.Link) linkRecord {
	return linkRecord{Code: l.Code, LongURL: l.LongURL}
}

// toDomain converts a GORM row back into the domain Link type.
func (rec linkRecord) toDomain() *link.Link {
	return &link.Link{Code: rec.Code, LongURL: rec.LongURL, CreatedAt: rec.CreatedAt}
}

// Repo implements link.Repository on top of GORM/Postgres.
type Repo struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Create(ctx context.Context, l *link.Link) error {
	rec := newRecord(l)

	if err := r.db.WithContext(ctx).Create(&rec).Error; err != nil {
		if isUniqueViolation(err) {
			return link.ErrCodeConflict
		}
		return fmt.Errorf("postgres: create link: %w", err)
	}

	l.CreatedAt = rec.CreatedAt
	return nil
}

func (r *Repo) GetByCode(ctx context.Context, code string) (*link.Link, error) {
	var rec linkRecord

	if err := r.db.WithContext(ctx).First(&rec, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, link.ErrNotFound
		}
		return nil, fmt.Errorf("postgres: get link by code: %w", err)
	}

	return rec.toDomain(), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
