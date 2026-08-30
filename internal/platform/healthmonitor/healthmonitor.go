// Package healthmonitor keeps a gRPC health.Server's reported status
// truthful after startup, not just at it. NewServer (coreservice/statservice)
// sets SERVING once, based only on the connections already being validated
// during boot — Run is what re-validates that periodically, so a Postgres
// outage that starts mid-run is actually reflected in the health check
// instead of the service reporting healthy forever after a good start.
package healthmonitor

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Checker reports whether a single dependency is currently reachable.
type Checker func(ctx context.Context) error

// Run periodically evaluates every checker and updates healthServer's
// overall ("") status — SERVING only if all of them succeed, NOT_SERVING
// otherwise. Deliberately scoped to hard dependencies only (a service
// literally cannot do its job without them, e.g. Postgres) — a soft
// dependency like a cache that already degrades gracefully on its own
// shouldn't flip the whole service unhealthy over it. Blocks until ctx is done.
func Run(ctx context.Context, healthServer *health.Server, interval time.Duration, log *zap.Logger, checks map[string]Checker) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			healthServer.SetServingStatus("", evaluate(ctx, interval, log, checks))
		}
	}
}

func evaluate(ctx context.Context, interval time.Duration, log *zap.Logger, checks map[string]Checker) healthpb.HealthCheckResponse_ServingStatus {
	status := healthpb.HealthCheckResponse_SERVING

	for name, check := range checks {
		checkCtx, cancel := context.WithTimeout(ctx, interval/2)
		err := check(checkCtx)
		cancel()

		if err != nil {
			log.Warn("health check: dependency unreachable", zap.String("dependency", name), zap.Error(err))
			status = healthpb.HealthCheckResponse_NOT_SERVING
		}
	}

	return status
}
