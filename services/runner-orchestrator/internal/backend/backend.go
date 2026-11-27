package backend

import (
	"context"
	"fmt"
)

const (
	// ConfigModeGRPC marks runner containers configured in-memory via WireRelay's gRPC Configure RPC
	// (no host bind-mounted WireGuard config file).
	ConfigModeGRPC = "grpc"
)

// RunnerSpec is completely backend-agnostic (docs/k8s-host.md section 3.1):
// no container IDs, no host paths, no IPs.
type RunnerSpec struct {
	SiteID          string
	Image           string
	ConfFingerprint string
	AuthToken       string
	DisableIPv6     bool
}

// RunnerState represents the observed runtime state of a site runner.
type RunnerState struct {
	SiteID          string
	Name            string
	RuntimeID       string
	Image           string
	ConfFingerprint string
	ConfigMode      string
	ObservedState   string // running | exited | missing | ...
}

// RunnerBackend is the K8s-portability seam implemented by DockerBackend in v1
// and swapped for client-go Deployments + headless Services + Secrets on Kubernetes.
type RunnerBackend interface {
	EnsureImage(ctx context.Context, imageRef string) error
	Ensure(ctx context.Context, spec RunnerSpec) (RunnerState, error)
	Stop(ctx context.Context, siteID string) error
	Remove(ctx context.Context, siteID string) error
	Get(ctx context.Context, siteID string) (RunnerState, error)
	List(ctx context.Context) ([]RunnerState, error)
}

// ContainerName returns the canonical runner hostname & container name.
func ContainerName(siteID string) string {
	return fmt.Sprintf("site-runner-%s", siteID)
}
