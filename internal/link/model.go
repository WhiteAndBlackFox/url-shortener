package link

import "time"

// Link represents a shortened URL record.
type Link struct {
	Code      string
	LongURL   string
	CreatedAt time.Time
}
