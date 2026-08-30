package httpapi

import (
	"net/http"
	"time"

	linkpb "URLShortener/api/proto/linkpb"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Handler exposes Core Service's link operations over public HTTP, reaching
// Core via gRPC. It is the transport-layer replacement for the Core
// Service's own httpapi.Handler from phases 1-3, now living in front of the
// gRPC boundary instead of the domain layer directly.
type Handler struct {
	client  linkpb.LinkServiceClient
	baseURL string
	log     *zap.Logger
}

func NewHandler(client linkpb.LinkServiceClient, baseURL string, log *zap.Logger) *Handler {
	return &Handler{client: client, baseURL: baseURL, log: log}
}

type createLinkRequest struct {
	URL string `json:"url" binding:"required"`
}

type linkResponse struct {
	Code      string    `json:"code"`
	ShortURL  string    `json:"short_url"`
	LongURL   string    `json:"long_url"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) newLinkResponse(l *linkpb.Link) linkResponse {
	return linkResponse{
		Code:      l.GetCode(),
		ShortURL:  h.baseURL + "/" + l.GetCode(),
		LongURL:   l.GetLongUrl(),
		CreatedAt: l.GetCreatedAt().AsTime(),
	}
}

// CreateLink handles POST /links.
func (h *Handler) CreateLink(c *gin.Context) {
	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	l, err := h.client.CreateLink(c.Request.Context(), &linkpb.CreateLinkRequest{Url: req.URL})
	if err != nil {
		h.handleRPCError(c, err)
		return
	}

	c.JSON(http.StatusCreated, h.newLinkResponse(l))
}

// Redirect handles GET /:code — the hot path that sends the client to the long URL.
func (h *Handler) Redirect(c *gin.Context) {
	code := c.Param("code")

	l, err := h.client.GetLink(c.Request.Context(), &linkpb.GetLinkRequest{Code: code})
	if err != nil {
		h.handleRPCError(c, err)
		return
	}

	c.Redirect(http.StatusFound, l.GetLongUrl())
}

// GetLinkInfo handles GET /links/:code.
func (h *Handler) GetLinkInfo(c *gin.Context) {
	code := c.Param("code")

	l, err := h.client.GetLink(c.Request.Context(), &linkpb.GetLinkRequest{Code: code})
	if err != nil {
		h.handleRPCError(c, err)
		return
	}

	c.JSON(http.StatusOK, h.newLinkResponse(l))
}

// handleRPCError translates a gRPC status error from Core Service into an
// HTTP response. Core's toStatusError (transport/grpc/link.go) puts the
// domain error message on InvalidArgument/NotFound, so those messages carry
// through to the client unchanged; anything else is hidden behind a generic
// message and logged here instead.
func (h *Handler) handleRPCError(c *gin.Context, err error) {
	st := status.Convert(err)

	switch st.Code() {
	case codes.InvalidArgument:
		c.JSON(http.StatusBadRequest, gin.H{"error": st.Message()})
	case codes.NotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": st.Message()})
	default:
		h.log.Error("core service rpc failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
