# WireDeck2

Centralized, multi-site industrial telemetry, remote browser VNC, and alerting platform built on **Go**, **PostgreSQL 18 (OLTP)**, **TimescaleDB (TSDB)**, **Vue 3**, and per-site **WireRelay** gRPC tunnel runners.

## Architecture Overview

- **Backend (100% Go):** `core-iam`, `runner-orchestrator`, `webvnc-proxy`, `webview-engine`, sharing a common `pkg/` layer (`uptrace/bun`, `pgx/v5`, `chi`, `huma/v2` OpenAPI 3.1 + strict JSON Schema validation).
- **Frontend (Vue 3 + TypeScript + Vite):** Three-surface modular SPA (`OperatorLayout`, lazy-loaded `SiteAdminLayout`, lazy-loaded `PlatformAdminLayout`).
- **Datastores:** Dedicated `postgres:18.7-alpine` (4 role-isolated OLTP schemas) + dedicated `timescale/timescaledb:2.32.1-pg18` (hypertables + continuous aggregates for sensor & register history).
- **In-Memory Caching:** `valkey` (in-memory key-value store) for shared caches and temporary data.
- **gRPC Tunnels:** `wirerelay` (per-site gRPC tunnel runner) for secure, low-latency, bidirectional communication with remote sites.
- **Email:** `mailgopher` a scalable gRCP email forwarder.
- **Network Isolation:** Segmented `wiredeck-control` (`internal: true`) and `wiredeck-runners` Docker bridges, dual-port public (`:8080`) vs internal (`:8081`) HTTP APIs, and Kubernetes-ready interfaces (`RunnerBackend`, `OwnershipProvider`, `LeaderElector`, `HistoryStore`).

## Documentation

- [deploy/PRODUCTION.md](./deploy/PRODUCTION.md) - Deployment & setup guide (Production, Staging, and Local)
- [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) - Authoritative platform architecture (v2.2)
- [docs/k8s-host.md](./docs/k8s-host.md) - Kubernetes evolution path & v1 seams
- [docs/WIREGUARD-CONF.md](./docs/WIREGUARD-CONF.md) - WireGuard client config rules & `AllowedIPs` narrowing
- [db/README.md](./db/README.md) - OLTP & TimescaleDB schema ownership and migration model

## Quick Start (Docker Compose)

See [deploy/PRODUCTION.md](./deploy/PRODUCTION.md) for full Production, Staging, and key-generation instructions.

### Local Development (Builds from Source)

```sh
# 1. Create local env file
cp deploy/.env.local.example deploy/.env.local
# Fill in JWT_PRIVATE_KEY_B64 and the 4 encryption/API keys (see deploy/PRODUCTION.md Step 4)

# 2. Build and start the local stack (App: http://localhost:8080, Mailpit: http://localhost:18025)
docker compose -f deploy/docker-compose.local.yml --env-file deploy/.env.local up -d --build
```

### Production (Prebuilt GHCR Images)

```sh
# 1. Create the two isolated Docker networks (control /24 + runners /22)
docker network create --driver bridge --internal --subnet=172.30.248.0/24 --ipv6=false wiredeck-control
docker network create --driver bridge --subnet=172.30.252.0/22 --ipv6=false wiredeck-runners

# 2. Configure environment and launch
cp deploy/.env.prod.example deploy/.env && chmod 600 deploy/.env
# Edit deploy/.env with generated secrets, WIREDECK_VERSION, domain, and SMTP_ACCOUNTS
docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env pull
docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env up -d
```
