package httpapi

import (
	"net/http"
	"time"

	"URLShortener/internal/platform/requestid"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery recovers from panics in downstream handlers, logs them, and
// responds with 500 instead of letting the process crash.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered",
					zap.Any("panic", rec),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			}
		}()
		c.Next()
	}
}

// RequestID is the origin of the request ID that flows through every hop in
// the system (see internal/platform/requestid): it reuses the caller's
// X-Request-Id header if present, otherwise mints a new one, puts it on the
// request context (so handlers can forward it over gRPC) and echoes it back
// on the response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestid.HTTPHeader)
		if id == "" {
			id = requestid.New()
		}
		c.Request = c.Request.WithContext(requestid.NewContext(c.Request.Context(), id))
		c.Writer.Header().Set(requestid.HTTPHeader, id)
		c.Next()
	}
}

// RequestLogger logs method, path, status, latency and the request ID for
// every request.
func RequestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		log.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.String("request_id", requestid.FromContext(c.Request.Context())),
			zap.Duration("latency", time.Since(start)),
		)
	}
}
