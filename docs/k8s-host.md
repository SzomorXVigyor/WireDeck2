# Kubernetes Evolution Path (v2.2)

**Status:** Docker Compose is the v1 deployment target (`docs/ARCHITECTURE.md`). Because all four backend services (`core-iam`, `runner-orchestrator`, `webvnc-proxy`, `webview-engine`) are written in **Go**, migrating to a multi-node Kubernetes cluster uses native `k8s.io/client-go` primitives with **zero service rewrites**.

---

## 1. What Ports Unchanged

- **Per-site network isolation:** Each runner Pod has its own network namespace, so overlapping customer LANs (`192.168.1.0/24`) continue to work unchanged. *(Never set `hostNetwork: true` on runners.)*
- **Stateless horizontal scaling:** `core-iam` and `webvnc-proxy` are stateless and scale horizontally out of the box (WebSockets are sticky for their session lifetime).
- **Traefik & nginx SPA:** Traefik v3 becomes a Kubernetes Ingress / `IngressRoute` controller; the Vue SPA nginx container becomes an ordinary Deployment.
- **Service-to-service calls:** All inter-service HTTP (`:8081`) and gRPC (`:9000`) calls use DNS names with zero shared filesystems.

---

## 2. What Changes on Kubernetes

### 2.1 Runner Provisioning (`RunnerBackend`)
- Docker SDK → `client-go`. Per active site: a 1-replica `Deployment`, a headless `Service`, and a `Secret` (holding the normalized `wg0.conf` + `WIRERELAY_AUTH_TOKEN`).
- DNS addressing stays identical:
  ```
  docker:      site-runner-<site_id>:9000
  kubernetes:  site-runner-<site_id>.wiredeck-runners.svc.cluster.local:9000
  ```
- **Config rotation improves:** Updating the `Secret` + `checksum/conf` Pod annotation triggers Kubernetes rollout; `runner-orchestrator` still calls `:8081/internal/sites/{siteId}/drain` beforehand.
- **Docker socket risk eliminated:** `/var/run/docker.sock` is replaced by a namespace-scoped `ServiceAccount` RBAC `Role` (Deployments, Services, Secrets in the runner namespace only).
- **Secret encryption caveat:** Enable Kubernetes etcd encryption-at-rest (or External Secrets / Vault) before migrating, since runner Secrets hold site private keys.

### 2.2 Pod Security, Kernel & Egress IP
- Runners require `CAP_NET_ADMIN` → place them in a dedicated `wiredeck-runners` namespace with `baseline` Pod Security Standard and a strict `NetworkPolicy` allowing ingress on `:9000` **only** from `runner-orchestrator`, `webview-engine`, and `webvnc-proxy`, and **denying all egress** to control-plane namespaces (`postgres-oltp`, `postgres-tsdb`, `core-iam`, `mailgopher`).
- Verify nodes ship the Linux kernel `wireguard` module (avoid slow userspace `wireguard-go` fallback).
- Add a `PriorityClass` and `PodDisruptionBudget` to runner Pods so rescheduling does not drop active site tunnels needlessly.
- **Stable Egress IP:** Customer firewalls that IP-allowlist WireGuard clients require an Egress Gateway or dedicated egress nodes with node affinity. Decide before onboarding IP-allowlisted sites.

### 2.3 `webview-engine` Scaling (`OwnershipProvider`)
1. **K1 (Lift):** Single-replica `Deployment` with `strategy: Recreate` (viable at 100 sites).
2. **K2 (Sharded HA Pollers):** Swap `SingleReplicaOwnership` for a native Go `client-go` **`coordination.k8s.io/v1` Lease** `OwnershipProvider` (one Lease per site; expired Leases are claimed automatically by healthy replicas).
3. **K3 (Browser-facing HA, if needed):** Add NATS between sharded pollers and a stateless WS gateway.

### 2.4 Leader-Elected Background Loops (`LeaderElector`)
All three singleton loops live in Go services and swap `NoopLeader` for native `client-go/tools/leaderelection`:
- `core-iam`: 15 s site status projection loop
- `runner-orchestrator`: 60 s reconciliation loop + 30 s health loop

### 2.5 Separated OLTP & TSDB Data Layer
- **OLTP (`postgres-oltp`):** Managed PostgreSQL 18 or CloudNativePG (`CNPG`) operator; add PgBouncer (CNPG `Pooler`) when scaling `core-iam` / `webvnc-proxy` to many replicas.
- **TSDB (`postgres-tsdb`):** Managed Timescale Cloud or dedicated TimescaleDB StatefulSet / CNPG cluster behind the `HistoryStore` Go interface.

---

## 3. Seams Built Into v1 (Zero-Cost Today)

1. **`RunnerBackend` (`runner-orchestrator`):** Backend-agnostic `Ensure / Stop / Remove / Get / List` taking `RunnerSpec{SiteID, Image, Conf, ConfFingerprint, AuthToken, DisableIPv6}` with zero Docker-specific fields.
2. **`OwnershipProvider` (`webview-engine`):** `OwnsSite(siteID) bool` + `OnOwnershipChanged(cb)` checked by poll scheduler, alarm evaluator, and history ingester.
3. **`LeaderElector` (`pkg/k8s`):** `RunAsLeader(ctx, name, fn)` wrapping all singleton background loops (`NoopLeader` in Docker Compose, `client-go` Lease on K8s).
4. **`HistoryStore` (`webview-engine`):** `WriteBatch / QueryRange / DeleteSite` encapsulating the dedicated TimescaleDB pool.
5. **`RESERVED_CIDRS` (`core-iam`):** Env-configured forbidden CIDRs (`172.30.248.0/24,172.30.252.0/22` on Docker Compose; cluster Pod + Service CIDRs on K8s).
