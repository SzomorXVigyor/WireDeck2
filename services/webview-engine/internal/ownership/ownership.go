package ownership

import "github.com/google/uuid"

// Provider abstracts site polling/alarm/ingest ownership (K8s seam, D4).
// In Docker Compose v1 SingleReplicaOwnership always returns true; on Kubernetes
// this swaps to coordination.k8s.io/v1 Lease-based sharding without touching
// the scheduler, alarm evaluator, or history ingester.
type Provider interface {
	OwnsSite(siteID uuid.UUID) bool
	OnOwnershipChanged(cb func(siteID uuid.UUID, owned bool))
}

// SingleReplicaOwnership owns all sites unconditionally (v1 Compose deployment).
type SingleReplicaOwnership struct{}

func (SingleReplicaOwnership) OwnsSite(_ uuid.UUID) bool                        { return true }
func (SingleReplicaOwnership) OnOwnershipChanged(_ func(uuid.UUID, bool))       {}
