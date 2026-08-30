package stats

import "context"

// Service contains the (thin) business logic around click statistics.
// It mainly exists to keep the same layering as link.Service: transport
// depends on Service, Service depends on Repository — not on a concrete
// store — so it stays testable with a fake repository.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// RecordClicks persists a batch of click events. Used by the worker pool,
// not by any HTTP/gRPC handler — batching happens before this is called.
func (s *Service) RecordClicks(ctx context.Context, events []ClickEvent) error {
	if len(events) == 0 {
		return nil
	}
	return s.repo.InsertBatch(ctx, events)
}

// GetStats returns click stats for a code. Unknown codes are not treated as
// an error: Stat Service has no way to know whether a code is a real short
// link (that's Core Service's concern) — it just reports zero clicks.
func (s *Service) GetStats(ctx context.Context, code string) (*Stats, error) {
	return s.repo.GetStats(ctx, code)
}
