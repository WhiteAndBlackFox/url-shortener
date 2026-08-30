package httpapi_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	statspb "URLShortener/api/proto/statspb"
	httpapi "URLShortener/internal/gateway/transport/http"
	platformpg "URLShortener/internal/platform/postgres"
	"URLShortener/internal/stats"
	statsrepo "URLShortener/internal/statservice/repository/postgres"
	statsgrpc "URLShortener/internal/statservice/transport/grpc"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// TestGetStats_ThroughGateway drives Gateway's /links/:code/stats route
// through a real gRPC client -> a real gRPC server (Stat Service, backed by
// real Postgres) -> the real stats.Service, over bufconn — the same
// technique link_integration_test.go uses for Core Service. RabbitMQ and the
// worker pool are bypassed on purpose: click ingestion is already covered by
// internal/statservice/worker's tests, so this test writes rows directly via
// the repository (simulating what a flushed batch would have done) to focus
// on the Gateway<->gRPC<->StatService<->Postgres read path.
func TestGetStats_ThroughGateway(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := statsrepo.New(db)
	code := "gwstats1"
	t.Cleanup(func() { db.Exec("DELETE FROM clicks WHERE code = ?", code) })

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.InsertBatch(ctx, []stats.ClickEvent{
		{EventID: "gwstats1-evt-1", Code: code, OccurredAt: now},
		{EventID: "gwstats1-evt-2", Code: code, OccurredAt: now.Add(time.Minute)},
	}))

	service := stats.NewService(repo)
	statsServer := statsgrpc.NewStatsServer(service, zap.NewNop())
	grpcServer, _ := statsgrpc.NewServer(statsServer, zap.NewNop())

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

	statsClient := statspb.NewStatsServiceClient(conn)
	statsHandler := httpapi.NewStatsHandler(statsClient, zap.NewNop())
	readinessHandler := httpapi.NewReadinessHandler(nil, nil) // not exercised by this test
	router := httpapi.NewRouter(httpapi.NewHandler(nil, "http://localhost:8080", zap.NewNop(), nil), statsHandler, readinessHandler, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/links/"+code+"/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code        string `json:"code"`
		TotalClicks int64  `json:"total_clicks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, code, resp.Code)
	require.Equal(t, int64(2), resp.TotalClicks)
}
