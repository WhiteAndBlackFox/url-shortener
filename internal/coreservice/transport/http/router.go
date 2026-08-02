package httpapi

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// NewRouter wires routes and middleware for the Core Service HTTP API.
//
// Route ordering note: "/links/:code" (static "links" segment + param) and
// "/:code" (a bare param at the root) can coexist — Gin's router matches
// static segments before falling back to a wildcard, so a request for
// "/links/abc" is routed to GetLinkInfo, not treated as Redirect with
// code="links".
func NewRouter(h *Handler, log *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(Recovery(log), RequestLogger(log))

	r.POST("/links", h.CreateLink)
	r.GET("/links/:code", h.GetLinkInfo)
	r.GET("/:code", h.Redirect)

	return r
}
