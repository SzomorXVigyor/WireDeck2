package wirerelay

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	pb "wiredeck/pkg/wirerelay/v1"
)

// RunnerAddress returns the canonical DNS address for a site's WireRelay runner.
// Works unchanged across Docker Compose (wiredeck-runners) and Kubernetes headless Services.
func RunnerAddress(siteID string) string {
	return fmt.Sprintf("site-runner-%s:9000", siteID)
}

// Dial opens a persistent gRPC client connection to a WireRelay runner with keepalive enabled.
func Dial(addr string) (*grpc.ClientConn, pb.WireRelayServiceClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, pb.NewWireRelayServiceClient(conn), nil
}

// WithAuthToken attaches the per-runner WIRERELAY_AUTH_TOKEN bearer metadata to ctx.
func WithAuthToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}
