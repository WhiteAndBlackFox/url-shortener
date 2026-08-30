package httpapi

import (
	_ "URLShortener/api/openapi" // side effect: registers the generated OpenAPI spec with swaggo

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

// NewRouter wires routes and middleware for the public Gateway HTTP API.
// This is now the ONLY public HTTP surface in the system — Core Service and
// Stat Service are both gRPC-only and unreachable directly by end users.
//
// Route ordering note: static routes ("/health", "/swagger", "/links/:code",
// etc.) and "/:code" (a bare param at the root) can coexist — Gin's router
// matches static segments before falling back to a wildcard, so e.g. a
// request for "/health" is routed to the health check, not treated as
// Redirect with code="health". "/links/:code/stats" is more specific still
// and matches before the two-segment "/links/:code" route.
func NewRouter(h *Handler, statsHandler *StatsHandler, log *zap.Logger) *gin.Engine {
	r := gin.New()
	r.Use(Recovery(log), RequestID(), RequestLogger(log))

	r.GET("/health", HealthCheck)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.POST("/links", h.CreateLink)
	r.GET("/links/:code", h.GetLinkInfo)
	r.GET("/links/:code/stats", statsHandler.GetStats)
	r.GET("/:code", h.Redirect)

	return r
}
