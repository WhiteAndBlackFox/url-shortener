package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter wires routes and middleware for the public Gateway HTTP API.
// This is now the ONLY public HTTP surface in the system — Core Service and
// Stat Service are both gRPC-only and unreachable directly by end users.
//
// Route ordering note: static routes ("/health", "/links/:code", etc.) and
// "/:code" (a bare param at the root) can coexist — Gin's router matches
// static segments before falling back to a wildcard, so e.g. a request for
// "/health" is routed to the health check, not treated as Redirect with
// code="health". "/links/:code/stats" is more specific still and matches
// before the two-segment "/links/:code" route.
func NewRouter(h *Handler, statsHandler *StatsHandler, log *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(Recovery(log), RequestID(), RequestLogger(log))

	// Liveness only (is the process up and able to serve HTTP) — deliberately
	// does not check Core/Stat Service reachability, so a downstream outage
	// doesn't cause Gateway itself to be marked unhealthy and restarted.
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.POST("/links", h.CreateLink)
	r.GET("/links/:code", h.GetLinkInfo)
	r.GET("/links/:code/stats", statsHandler.GetStats)
	r.GET("/:code", h.Redirect)

	return r
}
