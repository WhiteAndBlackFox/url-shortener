package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	linkpb "URLShortener/api/proto/linkpb"
	"URLShortener/internal/platform/requestid"
	"URLShortener/internal/stats"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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

	// publishWG tracks in-flight publishClickAsync goroutines. They're
	// detached from the request lifecycle on purpose (see
	// publishClickAsync), which means srv.Shutdown(ctx) alone doesn't wait
	// for them — WaitPublishers does, so main.go can hold off closing the
	// RabbitMQ publisher until every click that's still in flight has had a
	// chance to actually publish.
	publishWG sync.WaitGroup
}

func NewHandler(client linkpb.LinkServiceClient, baseURL string, log *zap.Logger, publisher clickPublisher) *Handler {
	return &Handler{client: client, baseURL: baseURL, log: log, publisher: publisher}
}

// WaitPublishers blocks until every in-flight click-publish goroutine has
// finished, or ctx is done — whichever comes first.
func (h *Handler) WaitPublishers(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		h.publishWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
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
		writeRPCError(c, h.log, err)
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
		writeRPCError(c, h.log, err)
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
	h.publishWG.Add(1)
	go func() {
		defer h.publishWG.Done()
		defer func() {
			if rec := recover(); rec != nil {
				h.log.Error("panic in click publish goroutine", zap.Any("panic", rec), zap.String("code", code))
			}
		}()

		ctx, cancel := context.WithTimeout(requestid.NewContext(context.Background(), requestID), 2*time.Second)
		defer cancel()

		// EventID is a fresh ID minted here, deliberately not requestID: a
		// caller-supplied X-Request-Id is untrusted and could be reused
		// across genuinely distinct requests, which would make it unsafe as
		// a dedup key (a repeat would look like a duplicate delivery and get
		// silently dropped by InsertBatch's ON CONFLICT DO NOTHING).
		ev := stats.ClickEvent{EventID: requestid.New(), Code: code, OccurredAt: time.Now(), IP: ip, UserAgent: userAgent}
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
		writeRPCError(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, h.newLinkResponse(l))
}
