package k8s

import "context"

// LeaderElector wraps singleton background loops (e.g., runner reconcile, health poll,
// site status projection). In Docker Compose v1 NoopLeader runs fn immediately;
// on Kubernetes it swaps cleanly to client-go/tools/leaderelection.
type LeaderElector interface {
	RunAsLeader(ctx context.Context, lockName string, fn func(ctx context.Context))
}

// NoopLeader assumes single-instance ownership (Docker Compose v1).
type NoopLeader struct{}

func (NoopLeader) RunAsLeader(ctx context.Context, _ string, fn func(ctx context.Context)) {
	fn(ctx)
}
