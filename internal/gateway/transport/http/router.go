package httpapi

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter wires routes and middleware for the public Gateway HTTP API.
// This is now the ONLY public HTTP surface in the system — Core Service and
// Stat Service are both gRPC-only and unreachable directly by end users.
//
// Route ordering note: "/links/:code" (static "links" segment + param) and
// "/:code" (a bare param at the root) can coexist — Gin's router matches
// static segments before falling back to a wildcard, so a request for
// "/links/abc" is routed to GetLinkInfo, not treated as Redirect with
// code="links". "/links/:code/stats" is more specific still and matches
// before the two-segment routes above.
func NewRouter(h *Handler, statsHandler *StatsHandler, log *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(Recovery(log), RequestLogger(log))

	r.POST("/links", h.CreateLink)
	r.GET("/links/:code", h.GetLinkInfo)
	r.GET("/links/:code/stats", statsHandler.GetStats)
	r.GET("/:code", h.Redirect)

	return r
}
