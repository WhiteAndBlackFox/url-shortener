package client

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial opens a gRPC connection to the Core Service.
//
// insecure.NewCredentials() means no TLS: acceptable here because this
// traffic never leaves the private docker-compose network. If Core Service
// ever became reachable from outside that network, this would need to
// switch to TLS/mTLS.
func Dial(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
