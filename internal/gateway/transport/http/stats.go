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
	Code          string    `json:"code" example:"abc1234"`
	TotalClicks   int64     `json:"total_clicks" example:"42"`
	LastClickedAt time.Time `json:"last_clicked_at,omitempty"`
}

// GetStats returns click statistics for a short link.
//
//	@Summary		Get click statistics
//	@Description	Returns the total click count and last-clicked time for a short code. Unknown codes report zero clicks rather than a 404 — Stat Service has no way to know whether a code is a real short link.
//	@Tags			stats
//	@Produce		json
//	@Param			code	path		string	true	"Short code"
//	@Success		200		{object}	statsResponse
//	@Failure		400		{object}	errorResponse
//	@Failure		500		{object}	errorResponse
//	@Router			/links/{code}/stats [get]
func (h *StatsHandler) GetStats(c *gin.Context) {
	code := c.Param("code")

	ctx := requestid.OutgoingContext(c.Request.Context())
	resp, err := h.client.GetStats(ctx, &statspb.GetStatsRequest{Code: code})
	if err != nil {
		writeRPCError(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, statsResponse{
		Code:          resp.GetCode(),
		TotalClicks:   resp.GetTotalClicks(),
		LastClickedAt: resp.GetLastClickedAt().AsTime(),
	})
}
