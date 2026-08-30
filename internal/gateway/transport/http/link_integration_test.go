package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	linkpb "URLShortener/api/proto/linkpb"
	postgresrepo "URLShortener/internal/coreservice/repository/postgres"
	coregrpc "URLShortener/internal/coreservice/transport/grpc"
	httpapi "URLShortener/internal/gateway/transport/http"
	"URLShortener/internal/link"
	platformpg "URLShortener/internal/platform/postgres"
	"URLShortener/internal/stats"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

// recordingPublisher is a fake for the Handler's clickPublisher dependency:
// no real RabbitMQ involved, it just records what it was asked to publish
// so tests can assert the redirect path fires a click event.
type recordingPublisher struct {
	mu     sync.Mutex
	events []stats.ClickEvent
}

func (p *recordingPublisher) Publish(_ context.Context, ev stats.ClickEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
	return nil
}

func (p *recordingPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

// newTestGatewayRouter drives the full split-service stack: a real Gin
// router (Gateway) -> a real gRPC client -> a real gRPC server (Core, backed
// by real Postgres) -> the real domain service. The gRPC server and client
// talk over bufconn (an in-memory net.Listener) instead of a TCP port, which
// is the standard way to integration-test gRPC code without binding real
// network sockets.
//
// Note: this bypasses the Redis cache decorator (that's Core-internal and
// already covered by internal/cache's own tests) and RabbitMQ (a fake
// publisher is used instead — the actual publish/consume pipeline is
// covered by internal/statservice/worker's tests) — the purpose here is to
// validate the Gateway<->gRPC<->Core<->Postgres path.
func newTestGatewayRouter(t *testing.T) (*gin.Engine, *gorm.DB, *recordingPublisher) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := postgresrepo.New(db)
	service := link.NewService(repo)
	linkServer := coregrpc.NewLinkServer(service, zap.NewNop())
	grpcServer, _ := coregrpc.NewServer(linkServer, zap.NewNop())

	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	bufDialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	linkClient := linkpb.NewLinkServiceClient(conn)
	pub := &recordingPublisher{}
	handler := httpapi.NewHandler(linkClient, "http://localhost:8080", zap.NewNop(), pub)
	statsHandler := httpapi.NewStatsHandler(nil, zap.NewNop()) // not exercised by these tests
	readinessHandler := httpapi.NewReadinessHandler(nil, nil)  // not exercised by these tests
	return httpapi.NewRouter(handler, statsHandler, readinessHandler, zap.NewNop()), db, pub
}

func TestCreateLink_ThenRedirectAndInfo(t *testing.T) {
	router, db, pub := newTestGatewayRouter(t)

	const longURL = "https://example.com/integration"

	body, err := json.Marshal(map[string]string{"url": longURL})
	require.NoError(t, err)

	createReq := httptest.NewRequest(http.MethodPost, "/links", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	require.NotEmpty(t, created.Code)

	t.Cleanup(func() { db.Exec("DELETE FROM links WHERE code = ?", created.Code) })

	redirectReq := httptest.NewRequest(http.MethodGet, "/"+created.Code, nil)
	redirectRec := httptest.NewRecorder()
	router.ServeHTTP(redirectRec, redirectReq)
	require.Equal(t, http.StatusFound, redirectRec.Code)
	require.Equal(t, longURL, redirectRec.Header().Get("Location"))

	require.Eventually(t, func() bool { return pub.count() == 1 }, time.Second, 10*time.Millisecond, "redirect must publish exactly one click event")

	infoReq := httptest.NewRequest(http.MethodGet, "/links/"+created.Code, nil)
	infoRec := httptest.NewRecorder()
	router.ServeHTTP(infoRec, infoReq)
	require.Equal(t, http.StatusOK, infoRec.Code)

	var info struct {
		Code    string `json:"code"`
		LongURL string `json:"long_url"`
	}
	require.NoError(t, json.Unmarshal(infoRec.Body.Bytes(), &info))
	require.Equal(t, created.Code, info.Code)
	require.Equal(t, longURL, info.LongURL)
}

func TestGetLinkInfo_NotFound(t *testing.T) {
	router, _, _ := newTestGatewayRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/links/doesnotexist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
