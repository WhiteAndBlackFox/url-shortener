// Package requestid carries a single request-scoped ID across every hop in
// the system: HTTP (Gateway) -> gRPC (Core/Stat Service) -> RabbitMQ
// (click events), purely for log correlation — grep one ID, see every
// service's log lines for that one user request.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"google.golang.org/grpc/metadata"
)

// HTTPHeader is the header name used on the public HTTP boundary.
const HTTPHeader = "X-Request-Id"

// Key is the wire-level key used everywhere that isn't the public HTTP
// boundary: gRPC metadata between Gateway and the internal services, and
// the AMQP message header Gateway attaches when publishing a click event.
// gRPC lowercases metadata keys, so this is written lowercase to begin with.
const Key = "x-request-id"

type ctxKey struct{}

// New generates a short random ID. Collision risk is irrelevant here (this
// is a tracing aid, not a security token), so a short id is enough to stay
// readable in logs.
func New() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf) // crypto/rand.Read essentially never errors on Linux
	return hex.EncodeToString(buf)
}

// NewContext returns a context carrying id, retrievable with FromContext.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the ID stored by NewContext, or "" if none.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// OutgoingContext attaches the context's request ID (if any) as gRPC
// metadata, so the callee's server-side interceptor can pick it up on the
// other end of the call.
func OutgoingContext(ctx context.Context) context.Context {
	id := FromContext(ctx)
	if id == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, Key, id)
}

// FromIncomingGRPC reads the request ID off incoming gRPC metadata, or ""
// if the call didn't carry one.
func FromIncomingGRPC(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(Key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
