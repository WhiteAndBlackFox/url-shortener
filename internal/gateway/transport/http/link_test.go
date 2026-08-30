package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	linkpb "URLShortener/api/proto/linkpb"
	statspb "URLShortener/api/proto/statspb"
	httpapi "URLShortener/internal/gateway/transport/http"
	"URLShortener/internal/platform/requestid"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeLinkClient is a hand-written fake of linkpb.LinkServiceClient — a
// small enough interface (2 methods) that a real double is simpler and more
// readable here than pulling in a mocking framework.
type fakeLinkClient struct {
	createFn func(ctx context.Context, in *linkpb.CreateLinkRequest) (*linkpb.Link, error)
	getFn    func(ctx context.Context, in *linkpb.GetLinkRequest) (*linkpb.Link, error)
}

func (f *fakeLinkClient) CreateLink(ctx context.Context, in *linkpb.CreateLinkRequest, _ ...grpc.CallOption) (*linkpb.Link, error) {
	return f.createFn(ctx, in)
}

func (f *fakeLinkClient) GetLink(ctx context.Context, in *linkpb.GetLinkRequest, _ ...grpc.CallOption) (*linkpb.Link, error) {
	return f.getFn(ctx, in)
}

type fakeStatsClient struct {
	getStatsFn func(ctx context.Context, in *statspb.GetStatsRequest) (*statspb.StatsResponse, error)
}

func (f *fakeStatsClient) GetStats(ctx context.Context, in *statspb.GetStatsRequest, _ ...grpc.CallOption) (*statspb.StatsResponse, error) {
	return f.getStatsFn(ctx, in)
}

// newTestRouter builds a router against fakes only — no Postgres, no
// bufconn, no RabbitMQ. Fast unit tests for the HTTP layer's own behavior
// (status code mapping, request bodies, request-id propagation), as opposed
// to link_integration_test.go, which exercises the real cross-service stack.
func newTestRouter(linkClient linkpb.LinkServiceClient, statsClient statspb.StatsServiceClient, pub *recordingPublisher) *gin.Engine {
	handler := httpapi.NewHandler(linkClient, "http://localhost:8080", zap.NewNop(), pub)
	statsHandler := httpapi.NewStatsHandler(statsClient, zap.NewNop())
	return httpapi.NewRouter(handler, statsHandler, zap.NewNop())
}

func TestHealthEndpoint(t *testing.T) {
	router := newTestRouter(&fakeLinkClient{}, &fakeStatsClient{}, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestID_GeneratedWhenAbsent(t *testing.T) {
	router := newTestRouter(&fakeLinkClient{}, &fakeStatsClient{}, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.NotEmpty(t, rec.Header().Get(requestid.HTTPHeader))
}

func TestRequestID_ReusedWhenProvided(t *testing.T) {
	router := newTestRouter(&fakeLinkClient{}, &fakeStatsClient{}, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(requestid.HTTPHeader, "caller-supplied-id")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, "caller-supplied-id", rec.Header().Get(requestid.HTTPHeader))
}

func TestCreateLink_Success(t *testing.T) {
	linkClient := &fakeLinkClient{
		createFn: func(_ context.Context, in *linkpb.CreateLinkRequest) (*linkpb.Link, error) {
			require.Equal(t, "https://example.com", in.GetUrl())
			return &linkpb.Link{Code: "abc1234", LongUrl: in.GetUrl(), CreatedAt: timestamppb.New(time.Now())}, nil
		},
	}
	router := newTestRouter(linkClient, &fakeStatsClient{}, &recordingPublisher{})

	body, _ := json.Marshal(map[string]string{"url": "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "abc1234", resp.Code)
	require.Equal(t, "http://localhost:8080/abc1234", resp.ShortURL)
}

func TestCreateLink_InvalidBody(t *testing.T) {
	router := newTestRouter(&fakeLinkClient{}, &fakeStatsClient{}, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateLink_UpstreamInvalidArgument(t *testing.T) {
	linkClient := &fakeLinkClient{
		createFn: func(context.Context, *linkpb.CreateLinkRequest) (*linkpb.Link, error) {
			return nil, status.Error(codes.InvalidArgument, "link: invalid url")
		},
	}
	router := newTestRouter(linkClient, &fakeStatsClient{}, &recordingPublisher{})

	body, _ := json.Marshal(map[string]string{"url": "not-a-url"})
	req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "link: invalid url")
}

func TestCreateLink_UpstreamInternalErrorHidesDetails(t *testing.T) {
	linkClient := &fakeLinkClient{
		createFn: func(context.Context, *linkpb.CreateLinkRequest) (*linkpb.Link, error) {
			return nil, status.Error(codes.Internal, "some sensitive db error detail")
		},
	}
	router := newTestRouter(linkClient, &fakeStatsClient{}, &recordingPublisher{})

	body, _ := json.Marshal(map[string]string{"url": "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "sensitive db error detail")
}

func TestRedirect_Success_PublishesClickEvent(t *testing.T) {
	linkClient := &fakeLinkClient{
		getFn: func(_ context.Context, in *linkpb.GetLinkRequest) (*linkpb.Link, error) {
			require.Equal(t, "abc1234", in.GetCode())
			return &linkpb.Link{Code: "abc1234", LongUrl: "https://example.com"}, nil
		},
	}
	pub := &recordingPublisher{}
	router := newTestRouter(linkClient, &fakeStatsClient{}, pub)

	req := httptest.NewRequest(http.MethodGet, "/abc1234", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://example.com", rec.Header().Get("Location"))
	require.Eventually(t, func() bool { return pub.count() == 1 }, time.Second, 10*time.Millisecond)
}

func TestRedirect_NotFound(t *testing.T) {
	linkClient := &fakeLinkClient{
		getFn: func(context.Context, *linkpb.GetLinkRequest) (*linkpb.Link, error) {
			return nil, status.Error(codes.NotFound, "link: not found")
		},
	}
	router := newTestRouter(linkClient, &fakeStatsClient{}, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/doesnotexist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStatsHandler_GetStats_Success(t *testing.T) {
	statsClient := &fakeStatsClient{
		getStatsFn: func(_ context.Context, in *statspb.GetStatsRequest) (*statspb.StatsResponse, error) {
			require.Equal(t, "abc1234", in.GetCode())
			return &statspb.StatsResponse{Code: "abc1234", TotalClicks: 7}, nil
		},
	}
	router := newTestRouter(&fakeLinkClient{}, statsClient, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/links/abc1234/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		TotalClicks int64 `json:"total_clicks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, int64(7), resp.TotalClicks)
}

func TestStatsHandler_GetStats_UpstreamErrorReturns500(t *testing.T) {
	statsClient := &fakeStatsClient{
		getStatsFn: func(context.Context, *statspb.GetStatsRequest) (*statspb.StatsResponse, error) {
			return nil, status.Error(codes.Unavailable, "stat service down")
		},
	}
	router := newTestRouter(&fakeLinkClient{}, statsClient, &recordingPublisher{})

	req := httptest.NewRequest(http.MethodGet, "/links/abc1234/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
