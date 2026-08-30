package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errorResponse is the JSON body returned for every non-2xx response.
type errorResponse struct {
	Error string `json:"error" example:"link: not found"`
}

// writeRPCError translates a gRPC status error from Core/Stat Service into
// an HTTP response — shared by Handler and StatsHandler so both map
// InvalidArgument/NotFound the same way instead of each hand-rolling (or,
// as StatsHandler used to, skipping) the mapping. Core/Stat's toStatusError
// puts the domain error message on InvalidArgument/NotFound, so those
// messages carry through to the client unchanged; anything else is hidden
// behind a generic message and logged here instead.
func writeRPCError(c *gin.Context, log *zap.Logger, err error) {
	st := status.Convert(err)

	switch st.Code() {
	case codes.InvalidArgument:
		c.JSON(http.StatusBadRequest, errorResponse{Error: st.Message()})
	case codes.NotFound:
		c.JSON(http.StatusNotFound, errorResponse{Error: st.Message()})
	default:
		log.Error("rpc failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}
