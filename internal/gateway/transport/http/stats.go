package httpapi

import (
	"net/http"
	"time"

	statspb "URLShortener/api/proto/statspb"
	"URLShortener/internal/platform/requestid"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// StatsHandler exposes Stat Service's click statistics over public HTTP,
// reaching Stat Service via gRPC — same pattern as Handler does for Core Service.
type StatsHandler struct {
	client statspb.StatsServiceClient
	log    *zap.Logger
}

func NewStatsHandler(client statspb.StatsServiceClient, log *zap.Logger) *StatsHandler {
	return &StatsHandler{client: client, log: log}
}

type statsResponse struct {
	Code          string    `json:"code"`
	TotalClicks   int64     `json:"total_clicks"`
	LastClickedAt time.Time `json:"last_clicked_at,omitempty"`
}

// GetStats handles GET /links/:code/stats.
func (h *StatsHandler) GetStats(c *gin.Context) {
	code := c.Param("code")

	ctx := requestid.OutgoingContext(c.Request.Context())
	resp, err := h.client.GetStats(ctx, &statspb.GetStatsRequest{Code: code})
	if err != nil {
		h.log.Error("stat service rpc failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, statsResponse{
		Code:          resp.GetCode(),
		TotalClicks:   resp.GetTotalClicks(),
		LastClickedAt: resp.GetLastClickedAt().AsTime(),
	})
}
