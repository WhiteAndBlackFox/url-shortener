package httpapi

import (
	"errors"
	"net/http"
	"time"

	"URLShortener/internal/link"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler exposes the Core Service's link operations over HTTP.
type Handler struct {
	service *link.Service
	baseURL string
	log     *zap.Logger
}

func NewHandler(service *link.Service, baseURL string, log *zap.Logger) *Handler {
	return &Handler{service: service, baseURL: baseURL, log: log}
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

func (h *Handler) newLinkResponse(l *link.Link) linkResponse {
	return linkResponse{
		Code:      l.Code,
		ShortURL:  h.baseURL + "/" + l.Code,
		LongURL:   l.LongURL,
		CreatedAt: l.CreatedAt,
	}
}

// CreateLink handles POST /links.
func (h *Handler) CreateLink(c *gin.Context) {
	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	l, err := h.service.CreateLink(c.Request.Context(), req.URL)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, h.newLinkResponse(l))
}

// Redirect handles GET /:code — the hot path that sends the client to the long URL.
func (h *Handler) Redirect(c *gin.Context) {
	code := c.Param("code")

	l, err := h.service.ResolveCode(c.Request.Context(), code)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.Redirect(http.StatusFound, l.LongURL)
}

// GetLinkInfo handles GET /links/:code.
func (h *Handler) GetLinkInfo(c *gin.Context) {
	code := c.Param("code")

	l, err := h.service.ResolveCode(c.Request.Context(), code)
	if err != nil {
		h.handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, h.newLinkResponse(l))
}

func (h *Handler) handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, link.ErrInvalidURL):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, link.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		h.log.Error("internal error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
