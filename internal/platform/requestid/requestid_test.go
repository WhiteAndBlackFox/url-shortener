package requestid_test

import (
	"context"
	"testing"

	"URLShortener/internal/platform/requestid"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestNew_ReturnsNonEmptyUniqueIDs(t *testing.T) {
	a, b := requestid.New(), requestid.New()
	require.NotEmpty(t, a)
	require.NotEqual(t, a, b)
}

func TestContext_RoundTrip(t *testing.T) {
	ctx := requestid.NewContext(context.Background(), "abc123")
	require.Equal(t, "abc123", requestid.FromContext(ctx))
}

func TestFromContext_EmptyWhenNotSet(t *testing.T) {
	require.Empty(t, requestid.FromContext(context.Background()))
}

func TestOutgoingContext_AttachesMetadata(t *testing.T) {
	ctx := requestid.NewContext(context.Background(), "abc123")
	ctx = requestid.OutgoingContext(ctx)

	md, ok := metadata.FromOutgoingContext(ctx)
	require.True(t, ok)
	require.Equal(t, []string{"abc123"}, md.Get("x-request-id"))
}

func TestOutgoingContext_NoopWhenNoID(t *testing.T) {
	ctx := requestid.OutgoingContext(context.Background())
	_, ok := metadata.FromOutgoingContext(ctx)
	require.False(t, ok)
}

func TestFromIncomingGRPC_ReadsMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "xyz789"))
	require.Equal(t, "xyz789", requestid.FromIncomingGRPC(ctx))
}

func TestFromIncomingGRPC_EmptyWhenAbsent(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
	require.Empty(t, requestid.FromIncomingGRPC(ctx))
}
