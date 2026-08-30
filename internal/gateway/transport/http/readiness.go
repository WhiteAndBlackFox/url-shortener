package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// ReadinessHandler checks whether Gateway can currently reach and get a
// healthy answer from Core Service and Stat Service, by calling their own
// gRPC health endpoints (the same standard protocol grpc-health-probe uses
// inside their containers — see coreservice/statservice's transport/grpc.NewServer).
// This is deliberately a separate endpoint from HealthCheck (/health):
// /health is pure liveness (is this process itself still running), so a
// downstream outage never causes Gateway to be killed and restarted over
// something a restart wouldn't fix; /ready is what actually reflects "can
// this instance currently do its job" for something like docker-compose's
// healthcheck to act on.
type ReadinessHandler struct {
	coreHealth healthpb.HealthClient
	statHealth healthpb.HealthClient
}

func NewReadinessHandler(coreHealth, statHealth healthpb.HealthClient) *ReadinessHandler {
	return &ReadinessHandler{coreHealth: coreHealth, statHealth: statHealth}
}

type readinessResponse struct {
	Core bool `json:"core"`
	Stat bool `json:"stat"`
}

// Ready reports readiness.
//
//	@Summary		Readiness check
//	@Description	Unlike /health (pure liveness), this actively checks Core Service and Stat Service's own gRPC health endpoints.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	readinessResponse
//	@Failure		503	{object}	readinessResponse
//	@Router			/ready [get]
func (h *ReadinessHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp := readinessResponse{
		Core: h.isServing(ctx, h.coreHealth),
		Stat: h.isServing(ctx, h.statHealth),
	}

	if resp.Core && resp.Stat {
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusServiceUnavailable, resp)
}

func (h *ReadinessHandler) isServing(ctx context.Context, client healthpb.HealthClient) bool {
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	return err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING
}
