package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"URLShortener/internal/coreservice/repository/postgres"
	httpapi "URLShortener/internal/coreservice/transport/http"
	"URLShortener/internal/link"
	platformpg "URLShortener/internal/platform/postgres"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestCreateLink_ThenRedirectAndInfo drives the full stack (real Postgres repo,
// service, Gin router) through the create -> redirect -> info flow. It also
// exercises the "/links/:code" vs "/:code" route-priority concern noted in router.go.
func TestCreateLink_ThenRedirectAndInfo(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	db, err := platformpg.NewDB(ctx, dsn)
	require.NoError(t, err)

	repo := postgres.New(db)
	service := link.NewService(repo)
	handler := httpapi.NewHandler(service, "http://localhost:8080", zap.NewNop())
	router := httpapi.NewRouter(handler, zap.NewNop())

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
