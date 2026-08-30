package stats

import "time"

// ClickEvent is the shared contract between the API Gateway (producer) and
// Stat Service (consumer): the Gateway JSON-encodes it onto the RabbitMQ
// queue on every successful redirect, Stat Service decodes it on the other
// end. It intentionally has no behavior — it's a data shape, not domain logic.
type ClickEvent struct {
	// EventID is a Gateway-generated, per-click unique ID — the idempotency
	// key that makes redelivery safe. RabbitMQ's at-least-once delivery
	// (combined with Nack(requeue=true) on a transient write failure, or a
	// timeout that fires right as a write actually succeeded) means the same
	// event can legitimately be processed more than once; InsertBatch relies
	// on a unique constraint on this column to make a duplicate a no-op
	// instead of a duplicate row.
	EventID    string    `json:"event_id"`
	Code       string    `json:"code"`
	OccurredAt time.Time `json:"occurred_at"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
}

// Stats is the aggregated click count for a single short link.
type Stats struct {
	Code          string
	TotalClicks   int64
	LastClickedAt time.Time
}
