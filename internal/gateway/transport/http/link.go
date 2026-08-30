package httpapi

import (
	"context"
	"net/http"
	"time"

	linkpb "URLShortener/api/proto/linkpb"
	"URLShortener/internal/platform/requestid"
	"URLShortener/internal/stats"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// clickPublisher is the narrow interface Handler needs from
// publisher.ClickPublisher — defined here (by the consumer), not there.
type clickPublisher interface {
	Publish(ctx context.Context, ev stats.ClickEvent) error
}

// Handler exposes Core Service's link operations over public HTTP, reaching
// Core via gRPC. It is the transport-layer replacement for the Core
// Service's own httpapi.Handler from phases 1-3, now living in front of the
// gRPC boundary instead of the domain layer directly.
type Handler struct {
	client    linkpb.LinkServiceClient
	baseURL   string
	log       *zap.Logger
	publisher clickPublisher
}

func NewHandler(client linkpb.LinkServiceClient, baseURL string, log *zap.Logger, publisher clickPublisher) *Handler {
	return &Handler{client: client, baseURL: baseURL, log: log, publisher: publisher}
}

type createLinkRequest struct {
	URL string `json:"url" binding:"required" example:"https://example.com/some/long/path"`
}

type linkResponse struct {
	Code      string    `json:"code" example:"abc1234"`
	ShortURL  string    `json:"short_url" example:"http://localhost:8080/abc1234"`
	LongURL   string    `json:"long_url" example:"https://example.com/some/long/path"`
	CreatedAt time.Time `json:"created_at"`
}

// errorResponse is the JSON body returned for every non-2xx response.
type errorResponse struct {
	Error string `json:"error" example:"link: not found"`
}

func (h *Handler) newLinkResponse(l *linkpb.Link) linkResponse {
	return linkResponse{
		Code:      l.GetCode(),
		ShortURL:  h.baseURL + "/" + l.GetCode(),
		LongURL:   l.GetLongUrl(),
		CreatedAt: l.GetCreatedAt().AsTime(),
	}
}

// CreateLink creates a short link.
//
//	@Summary		Create a short link
//	@Description	Validates the URL and returns a newly created short code.
//	@Tags			links
//	@Accept			json
//	@Produce		json
//	@Param			request	body		createLinkRequest	true	"URL to shorten"
//	@Success		201		{object}	linkResponse
//	@Failure		400		{object}	errorResponse	"invalid request body or URL"
//	@Failure		500		{object}	errorResponse
//	@Router			/links [post]
func (h *Handler) CreateLink(c *gin.Context) {
	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	ctx := requestid.OutgoingContext(c.Request.Context())
	l, err := h.client.CreateLink(ctx, &linkpb.CreateLinkRequest{Url: req.URL})
	if err != nil {
		h.handleRPCError(c, err)
		return
	}

	c.JSON(http.StatusCreated, h.newLinkResponse(l))
}

// Redirect sends the client to the long URL behind a short code.
//
//	@Summary		Redirect to the long URL
//	@Description	The hot path: resolves a short code and redirects the client. Also publishes a click event to RabbitMQ, asynchronously.
//	@Tags			links
//	@Param			code	path	string	true	"Short code"
//	@Success		302
//	@Failure		404	{object}	errorResponse
//	@Router			/{code} [get]
func (h *Handler) Redirect(c *gin.Context) {
	code := c.Param("code")

	ctx := requestid.OutgoingContext(c.Request.Context())
	l, err := h.client.GetLink(ctx, &linkpb.GetLinkRequest{Code: code})
	if err != nil {
		h.handleRPCError(c, err)
		return
	}

	h.publishClickAsync(requestid.FromContext(c.Request.Context()), code, c.ClientIP(), c.Request.UserAgent())
	c.Redirect(http.StatusFound, l.GetLongUrl())
}

// publishClickAsync fires the click event on its own goroutine with a short,
// independent timeout, so a slow or unavailable RabbitMQ never delays the
// redirect response the client is waiting on. context.Background() (not
// c.Request.Context()) is the base here on purpose: the request context is
// canceled once the HTTP response is written, which would race with (and
// likely abort) this publish — requestID is threaded through explicitly
// since it's the one piece of the request context still needed downstream.
// A deferred recover keeps a bug in this path from crashing the whole
// process — it runs detached from any request-scoped recovery middleware.
func (h *Handler) publishClickAsync(requestID, code, ip, userAgent string) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				h.log.Error("panic in click publish goroutine", zap.Any("panic", rec), zap.String("code", code))
			}
		}()

		ctx, cancel := context.WithTimeout(requestid.NewContext(context.Background(), requestID), 2*time.Second)
		defer cancel()

		ev := stats.ClickEvent{Code: code, OccurredAt: time.Now(), IP: ip, UserAgent: userAgent}
		if err := h.publisher.Publish(ctx, ev); err != nil {
			h.log.Error("publish click event failed", zap.String("code", code), zap.Error(err))
		}
	}()
}

// GetLinkInfo returns metadata about a short link.
//
//	@Summary		Get link info
//	@Description	Returns the short code, long URL and creation time — does not redirect.
//	@Tags			links
//	@Produce		json
//	@Param			code	path		string	true	"Short code"
//	@Success		200		{object}	linkResponse
//	@Failure		404		{object}	errorResponse
//	@Router			/links/{code} [get]
func (h *Handler) GetLinkInfo(c *gin.Context) {
	code := c.Param("code")

	ctx := requestid.OutgoingContext(c.Request.Context())
	l, err := h.client.GetLink(ctx, &linkpb.GetLinkRequest{Code: code})
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
		c.JSON(http.StatusBadRequest, errorResponse{Error: st.Message()})
	case codes.NotFound:
		c.JSON(http.StatusNotFound, errorResponse{Error: st.Message()})
	default:
		h.log.Error("core service rpc failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}
