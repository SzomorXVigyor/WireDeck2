# WireDeck2

Centralized, multi-site industrial telemetry, remote browser VNC, and alerting platform built on **Go**, **PostgreSQL 18 (OLTP)**, **TimescaleDB (TSDB)**, **Vue 3**, and per-site **WireRelay** gRPC tunnel runners.

## Architecture Overview

- **Backend (100% Go):** `core-iam`, `runner-orchestrator`, `webvnc-proxy`, `webview-engine`, sharing a common `pkg/` layer (`uptrace/bun`, `pgx/v5`, `chi`, `huma/v2` OpenAPI 3.1 + strict JSON Schema validation).
- **Frontend (Vue 3 + TypeScript + Vite):** Three-surface modular SPA (`OperatorLayout`, lazy-loaded `SiteAdminLayout`, lazy-loaded `PlatformAdminLayout`).
- **Datastores:** Dedicated `postgres:18-alpine` (4 role-isolated OLTP schemas) + dedicated `timescale/timescaledb:2.32.0-pg17` (hypertables + continuous aggregates for sensor & register history).
- **Site Runners & Email:** Consumes hardened [WireRelay](../WireRelay) (`v1.1.0+`) and [MailGopher](../MailGopher) (`v1.2.0+`) images.
- **Network Isolation:** Segmented `wiredeck-control` (`internal: true`) and `wiredeck-runners` Docker bridges, dual-port public (`:8080`) vs internal (`:8081`) HTTP APIs, and Kubernetes-ready interfaces (`RunnerBackend`, `OwnershipProvider`, `LeaderElector`, `HistoryStore`).

## Documentation

- [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) — Authoritative platform architecture (v2.2)
- [docs/k8s-host.md](./docs/k8s-host.md) — Kubernetes evolution path & v1 seams
- [docs/WIREGUARD-CONF.md](./docs/WIREGUARD-CONF.md) — WireGuard client config rules & `AllowedIPs` narrowing
- [db/README.md](./db/README.md) — OLTP & TimescaleDB schema ownership and migration model

## Quick Start (Docker Compose)

```sh
# 1. Create the two isolated Docker networks (control /24 + runners /22)
docker network create --driver bridge --internal --subnet=172.30.248.0/24 --ipv6=false wiredeck-control
docker network create --driver bridge --subnet=172.30.252.0/22 --ipv6=false wiredeck-runners

# 2. Mount tmpfs for ephemeral runner WireGuard configs (private keys never touch disk)
sudo mkdir -p /var/lib/wiredeck2/runners
sudo mount -t tmpfs -o size=16m,mode=0700 tmpfs /var/lib/wiredeck2/runners

# 3. Configure environment
cp deploy/.env.example deploy/.env
# Edit deploy/.env with generated secrets and domain

# 4. Launch production stack (or add -f deploy/docker-compose.dev.yml for local dev + simulators)
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d
```
