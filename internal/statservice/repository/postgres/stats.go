package postgres

import (
	"context"
	"fmt"
	"time"

	"URLShortener/internal/stats"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// clickRecord is the GORM-mapped row for the "clicks" table, kept private
// and separate from stats.ClickEvent — same reasoning as coreservice's
// link/postgres.go: the domain package stays free of ORM tags.
type clickRecord struct {
	ID         uint      `gorm:"column:id;primaryKey"`
	EventID    string    `gorm:"column:event_id"`
	Code       string    `gorm:"column:code;size:16;not null"`
	OccurredAt time.Time `gorm:"column:occurred_at;not null"`
	IP         string    `gorm:"column:ip"`
	UserAgent  string    `gorm:"column:user_agent"`
}

func (clickRecord) TableName() string {
	return "clicks"
}

func newRecord(e stats.ClickEvent) clickRecord {
	return clickRecord{EventID: e.EventID, Code: e.Code, OccurredAt: e.OccurredAt, IP: e.IP, UserAgent: e.UserAgent}
}

// Repo implements stats.Repository on top of GORM/Postgres.
type Repo struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// InsertBatch writes all events in a single multi-row INSERT. Called by the
// worker pool after it has accumulated a batch — never one event at a time.
//
// ON CONFLICT (event_id) DO NOTHING makes redelivery idempotent: RabbitMQ's
// at-least-once delivery means the same event can be processed more than
// once (a Nack(requeue=true) after a transient write failure, or a flush
// timeout that fires right as the write actually committed) — without this,
// that redelivery would silently double-count a click.
func (r *Repo) InsertBatch(ctx context.Context, events []stats.ClickEvent) error {
	records := make([]clickRecord, len(events))
	for i, e := range events {
		records[i] = newRecord(e)
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).
		Create(&records).Error
	if err != nil {
		return fmt.Errorf("postgres: insert click batch: %w", err)
	}
	return nil
}

func (r *Repo) GetStats(ctx context.Context, code string) (*stats.Stats, error) {
	var row struct {
		Total int64
		Last  *time.Time
	}

	err := r.db.WithContext(ctx).
		Model(&clickRecord{}).
		Where("code = ?", code).
		Select("COUNT(*) AS total, MAX(occurred_at) AS last").
		Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("postgres: get stats: %w", err)
	}

	result := &stats.Stats{Code: code, TotalClicks: row.Total}
	if row.Last != nil {
		result.LastClickedAt = *row.Last
	}
	return result, nil
}
