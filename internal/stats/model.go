package stats

import "time"

// ClickEvent is the shared contract between the API Gateway (producer) and
// Stat Service (consumer): the Gateway JSON-encodes it onto the RabbitMQ
// queue on every successful redirect, Stat Service decodes it on the other
// end. It intentionally has no behavior — it's a data shape, not domain logic.
type ClickEvent struct {
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
