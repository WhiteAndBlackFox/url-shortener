package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck reports process liveness.
//
//	@Summary		Health check
//	@Description	Liveness only — deliberately does not check Core/Stat Service reachability, so a downstream outage doesn't cause Gateway itself to be marked unhealthy and restarted.
//	@Tags			health
//	@Success		200
//	@Router			/health [get]
func HealthCheck(c *gin.Context) {
	c.Status(http.StatusOK)
}
