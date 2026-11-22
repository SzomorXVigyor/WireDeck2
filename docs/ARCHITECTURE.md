# WireDeck2 — Platform Architecture Specification

**Version:** 2.2 · **Date:** 2026-11-22 · **Status:** Approved & Locked for Implementation

Supersedes v2.0 (2026-08-09) and the pilot specification. This document is the single architectural authority for WireDeck2.

WireDeck2 is a greenfield rewrite of WireDeck. It replaces the *discrete site runner* model (one `wg-easy` + dedicated `webvnc` and `webview` container stack per site) with **aggregated Go microservices reaching many sites through per-site WireRelay gRPC tunnel runners**, backed by **separated PostgreSQL 18 (OLTP) and TimescaleDB (TSDB)** datastores and a **Three-Surface Vue 3 SPA**.

---

## 0. Decision Log

| # | Decision | Rationale |
|---|---|---|
| D1 | WireGuard **server is external to the platform**. Sites run their own. | Removes VPN server/peer key lifecycle from platform. Sites need a reachable UDP endpoint (static IP or DDNS + port forward). |
| D2 | On site creation an admin **uploads a WireGuard client config**. | Platform is a WireGuard *client*. Config is validated, narrowed, normalised and AES-256-GCM encrypted at rest. |
| D3 | No end-user VPN client configs. **Browser access only.** | `wg-easy` is gone. Users access sites through browser VNC and WebView only. |
| D4 | `webview-engine` runs **single-replica** in v1 (with `OwnershipProvider` K8s seam). | No Redis, no message bus, no leader election in v1 Compose; Lease-based site sharding on K8s later. |
| D5 | Separate **`runner-orchestrator`** owns `/var/run/docker.sock` behind `RunnerBackend`. | Keeps host-equivalent Docker access off internet-facing services; swaps to `client-go` on K8s. |
| D6 | Scale target **25–100 sites**, ~20 devices/site, ~5,000 register mappings. | Per-device stream pooling and contiguous register coalescing are mandatory. |
| D7 | **Strictly three platform roles**: **Platform Admin** (`is_platform_admin`), **Site Operator** (`SITE_OPERATOR`), and **Site Member** (`SITE_MEMBER`). | Platform Admin is sole overlord of sites, site features (`vnc_enabled`, `webview_enabled`), WireGuard configs, runners, and global users. Site Operator manages site members (inviting new emails as `SITE_MEMBER`) and service-level config (VNC edge points, WebView pages/elements/history/alarms). Site Member uses enabled services; fine-grained policies live inside each service. |
| D8 | **VPN Portal (iframe) dropped from v1.** | High security surface, low value. |
| D9 | History uses **deadband + max-interval**, not store-every-poll. | 1–2 orders of magnitude less write/storage volume; charts use TimescaleDB `time_bucket_gapfill` + `locf()`. |
| D10 | Alarms **latch and re-arm**: fire once, silent clear (or opt-in email), re-arm on return beyond `clear_deadband`. Ack is cosmetic. | Prevents alert storms; persisted `alarm_pending_states` preserves holds across restarts. |
| D11 | **No MFA in v1.** | Email + argon2id password, rotating refresh cookie, admin email invites, anytime password change/reset. |
| D12 | On site `OFFLINE`: **freeze active alarms (`is_stale = true`), cancel pending timers**. | Never raise an alarm from unread/stale data. Driven by `core-iam` runtime-state sync (D28). |
| D13 | Retention: **raw 30d, continuous aggregates (`1m`, `1h`) 1y**. History storage is **part of WebView** (not a separate site-level feature flag). | Configured inside `webview-engine` whenever `webview_enabled = true`. |
| D14 | VNC: **no session locking** — target VNC server decides. Idle timeout (`15m`) & max session (`8h`) enforced. | Releases runner gRPC streams and sockets cleanly. |
| D15 | Modbus writes go over **REST** (`POST /write`), not WebSocket. | Explicit request/response, RBAC check, and error feedback against physical outputs. |
| D16 | **No audit log table in v1.** | Writes emit structured JSON audit logs (`user_id`, `site_id`, `mapping_id`, `value`, `result`). |
| D17 | Polling is **demand-driven** across 3 demand sources: archival (`history_enabled`), alarm (active rules), interactive (WS subscribers). | Mappings with zero demand are never polled. |
| D18 | `HistoryChart` renders even when `history_enabled = false`, backed by a **900-pt client-side live ring buffer**. | Deep-dive `/history` route requires `history_enabled`; dashboard chart widget always works. |
| D19 | Modbus PDUs are **constructed server-side** in `webview-engine`. | WireRelay stays a dumb protocol-agnostic TCP/UDP byte pipe. |
| **D20** | **Segmented Docker networks**: `wiredeck-control` (`internal: true`) vs `wiredeck-runners`. | Compromised site runners or LAN route leaks physically cannot reach Postgres, TimescaleDB, `core-iam`, `mailgopher`, or Traefik. |
| **D21** | **Dual-port public (`:8080`) vs internal (`:8081`) HTTP APIs** + `INTERNAL_API_TOKEN`. | Structurally impossible for Traefik to expose `/internal/*`. `runner-orchestrator` binds strictly on `wiredeck-control`. |
| **D22** | **Reboot-safe tmpfs recovery**: `runner-orchestrator` pulls desired runner specs & missing confs from `core-iam:8081`. | Private keys remain on tmpfs (never hit disk) while surviving host reboots automatically. |
| **D23** | **Separate AES-256-GCM encryption keys** per service (`WG_CONF_ENCRYPTION_KEY` vs `VNC_PASSWORD_ENCRYPTION_KEY`). | Compromising `webvnc-proxy` cannot decrypt site WireGuard private keys. |
| **D24** | **All-Go backend** (`Go 1.26/1.27` + `uptrace/bun` + `pgx/v5` + `chi` + `huma/v2`). | Shared `pkg/` (gRPC, JWKS, crypto, telemetry), native `client-go` K8s Leases, 10× lower RAM, strict schema validation + OpenAPI 3.1/Swagger. |
| **D25** | **Server-side RFB 3.8 VNC authentication** in `webvnc-proxy`. | Proxy completes DES challenge-response with the VNC server using `password_encrypted`; VNC passwords never reach the browser. |
| **D26** | **Native Go MBAP + Modbus PDU codec** directly over `WireRelay.TunnelStream` (incl. FC `0x10`). | Supports 16-bit & 32-bit (`UINT32`/`INT32`/`FLOAT32`) reads and writes with configurable word/byte order. |
| **D27** | **Full WireDeck v1 widget & register parity**: `lamp`, `display`, `switch`, `button`, `number_input`, `chart`, plus `access_mode` (`READ \| WRITE \| READ_WRITE`). | Preserves industrial setpoint writes, momentary commands, and write-only registers. |
| **D28** | **Cross-service lifecycle contracts**: `GET /internal/sites/runtime-state` (15 s poll), `POST /internal/sites/:id/drain`, `DELETE /internal/sites/:id/data`. | Powers D12 offline alarm freeze, live flag toggles, graceful conf rotation, and clean site deletion cascades. |
| **D29** | **Separated OLTP (`postgres:18-alpine`) + TSDB (`timescale/timescaledb:2.32.0-pg17`)** without PgBouncer in v1, behind `HistoryStore`. | Isolates sensor write/rollup I/O from OLTP auth/config, allows trivial OLTP `pg_dump`, preserves prepared statements. |
| **D30** | **Admin email-invite user creation + anytime self-service password change/reset** + **Strict `/admin/*` API & Three-Surface SPA separation**. | Admin invites via `MailGopher` (`user_tokens`), rotating `HttpOnly` refresh cookies, router-enforced admin groups, and clean Operator / Site-Admin / Platform-Admin UI shells. |

---

## 1. Scope & Non-Functional Targets

- **In scope (v1):** Identity, admin email invites, password change/reset, per-site RBAC · site onboarding via WG client-config upload & `AllowedIPs` narrowing · automated runner provisioning, reconciliation & health monitoring · server-side-authenticated browser VNC · demand-driven Modbus TCP/UDP polling & REST writes · TimescaleDB sensor/register history with per-site toggle · latch/re-arm Alarm Centre with MailGopher alerts · OpenAPI 3.1 / Swagger docs.
- **Out of scope (v1):** WireGuard server hosting · end-user VPN configs (D3) · embedded VPN portal (D8) · MFA (D11) · audit table (D16) · multi-tenancy above site level · migration from WireDeck 1.x.

| Target | Value |
|---|---|
| Sites | 100 |
| Devices per site | ~20 |
| Register mappings total | ~5,000 |
| Interactive poll latency | ≤ 1 s end to end |
| Runner containers on one host | 100 |
| Acceptable `webview-engine` restart downtime | ≤ 30 s, zero alarm or buffered history loss |

---

## 2. Topology & Network Boundaries

```
                        ┌────────────────────────────────────────────────────┐
                        │   Vue 3.5 SPA (Operator / Site Admin / Platform)   │
                        └──────────────────────────┬─────────────────────────┘
                                                   │ HTTPS / WSS
                                                   v
                        ┌────────────────────────────────────────────────────┐
                        │       Traefik v3.7 (TLS, headers, rate limit)      │
                        └──┬──────────────┬───────────────┬──────────────┬───┘
             /api/v1/iam   │ /api/v1/     │ /api/v1/vnc   │              │ /
                 (:8080)   │ webview      │   (:8080)     │              v
                           v (:8080)      v               │        ┌───────────┐
   ┌───────────────────────── wiredeck-control (internal) ─────────│   nginx   │
   │  ┌─────────────┐  ┌──────────┐  ┌──────────┐                  │   (SPA)   │
   │  │  core-iam   │  │ webview  │  │  webvnc  │                  └───────────┘
   │  │ (Go + Bun)  │  │ -engine  │  │  -proxy  │
   │  └──────┬──────┘  │(Go + Bun)│  │(Go + Bun)│
   │         │ :8081   └────┬─────┘  └────┬─────┘
   │         v              │ :8081       │ :8081
   │  ┌──────────────┐      │             │       ┌─────────────────────────────┐
   │  │   runner-    │<─────┴─────────────┘       │ mailgopher (internal only)  │
   │  │ orchestrator │                            └─────────────────────────────┘
   │  │     (Go)     │    ┌─────────────────────┐ ┌─────────────────────────────┐
   │  └──┬────────┬──┘    │ PostgreSQL 18 (OLTP)│ │ TimescaleDB 2.32-pg17 (TSDB)│
   └─────┼────────┼───────┴─────────────────────┴─┴─────────────────────────────┘
         │        │
   docker│        │ gRPC (:9000 + WIRERELAY_AUTH_TOKEN)
   .sock │        │               ┌── wiredeck-runners (isolated data bridge) ──┐
         v        └──────────────>│  site-runner-<site_id>                      │
  ┌────────────┐   creates (:ro)  │  (WireRelay v1.1+ / wg0 / ip_forward=0)     │
  │ Docker API ├─────────────────>└──────────────────────┬──────────────────────┘
  └────────────┘                                         │ WireGuard UDP
                                                         v
                                  ┌─────────────────────────────────────────────┐
                                  │  Customer Site LAN (PLCs, Sensors, VNC)     │
                                  └─────────────────────────────────────────────┘
```

**Hard Invariants:**
- Site runners sit **only** on `wiredeck-runners` with `ip_forward=0` and `FORWARD DROP`; they cannot route to Postgres, TimescaleDB, `core-iam`, `mailgopher`, or Traefik.
- `runner-orchestrator` binds its HTTP API strictly on its `wiredeck-control` IP.
- Internal APIs (`:8081/internal/*`) require `Authorization: Bearer <INTERNAL_API_TOKEN>` and are never routed by Traefik.
- Runner ports are never published to the host.
- Every OLTP service connects with its own Postgres role owning strictly its own schema.

---

## 3. Repository Layout

```
WireDeck2/
├── docs/
│   ├── ARCHITECTURE.md          # this specification (v2.2)
│   ├── k8s-host.md              # Kubernetes evolution & seams
│   ├── WIREGUARD-CONF.md        # admin-facing WG conf & narrowing guide
│   └── RUNBOOK.md               # Phase 4 ops & backup/restore drills
├── pkg/                         # Shared Go packages (api, auth, crypto, wireguard, wirerelay, telemetry, k8s)
├── services/
│   ├── core-iam/                # Go + Bun + Huma v2 + embedded goose migrations
│   ├── runner-orchestrator/     # Go + Bun + Docker RunnerBackend + embedded goose migrations
│   ├── webvnc-proxy/            # Go + Bun + Huma v2 + RFB 3.8 proxy + embedded goose migrations
│   └── webview-engine/          # Go + Bun + Huma v2 + MBAP codec + TSDB HistoryStore + Alarms
├── frontend/                    # Vue 3.5 + Vite 8 + TS + Pinia + Tailwind v4 (Three-Surface SPA)
├── db/
│   ├── README.md
│   ├── oltp/                    # PG18 bootstrap migrator (extensions, 4 schemas, 4 roles)
│   └── tsdb/                    # TimescaleDB migrator (hypertable, compression, 1m/1h CAGGs, retention)
├── deploy/
│   ├── docker-compose.yml       # production single-host stack
│   ├── docker-compose.dev.yml   # local dev + Modbus/VNC simulators
│   ├── .env.example
│   ├── traefik/                 # traefik.yml + dynamic.yml
│   └── nginx/                   # default.conf for SPA
└── .github/workflows/           # build-all.yml + release-<service>.yml
```

---

## 4. HTTP API Standards, Schema Validation & Admin Separation

Every public HTTP service (`core-iam`, `webview-engine`, `webvnc-proxy`) uses **`go-chi/chi/v5` + `danielgtaylor/huma/v2`**:
- **100% Schema-Validated:** All path, query, header, and JSON body inputs are strictly validated via JSON Schema (`additionalProperties: false`, explicit types/ranges/formats/enums), returning RFC 9457 Problem Details on validation failure.
- **Auto-Generated OpenAPI 3.1 + Interactive Swagger/Scalar UI:** Served at `/api/v1/<service>/openapi.json` and `/api/v1/<service>/docs` (controlled via `ENABLE_OPENAPI_DOCS`), used to generate typed TypeScript clients (`openapi-typescript`) for `frontend/`.
- **Strict Router-Group Separation:**
  - **Public (Unauth):** `/api/v1/iam/auth/*` (login, cookie refresh, accept-invite, forgot/reset password), `/api/v1/iam/.well-known/jwks.json`.
  - **Self-Service User:** `/api/v1/iam/me/*` (`RequireAuth`: profile, change-password, accessible sites).
  - **Site Member / Service Consumer:** `/api/v1/webview/sites/{siteId}/*`, `/api/v1/vnc/sites/{siteId}/*`, `/api/v1/webview/alarms` (`RequireAuth` + `RequireSiteMember`; fine-grained write/view policies live inside `webview-engine`).
  - **Site Operator:** `/api/v1/iam/sites/{siteId}/operator/members*` (list, invite new email as `SITE_MEMBER`, remove `SITE_MEMBER`), `/api/v1/webview/sites/{siteId}/operator/*` (devices, registers, dashboards, history settings, alarm rules), `/api/v1/vnc/sites/{siteId}/operator/*` (VNC edge points) (`RequireAuth` + `RequireSiteOperator`). **Site Operators cannot access WireGuard configs or site feature flags.**
  - **Platform Admin:** `/api/v1/iam/admin/*` (`RequireAuth` + `RequirePlatformAdmin` — sole owner of site creation/deletion/activation, `vnc_enabled`/`webview_enabled` toggles, WireGuard conf upload/rotation, global user management, `SITE_OPERATOR` assignment, and runners).
  - **Internal Control Plane:** `:8081/internal/*` (`RequireInternalToken`, never exposed to Traefik or public OpenAPI).

---

## 5. Data Architecture

### 5.1 Separated Datastores

| Instance | Image | Schemas & Owners | Migrations |
|---|---|---|---|
| `postgres-oltp` | `postgres:18-alpine` | `platform_iam` (`svc_core_iam`) · `platform_runners` (`svc_runner_orchestrator`) · `platform_webvnc` (`svc_webvnc_proxy`) · `platform_webview` (`svc_webview_engine`) | Bootstrap via `db/oltp/`; tables via embedded `goose` per service |
| `postgres-tsdb` | `timescale/timescaledb:2.32.0-pg17` | `platform_webview_history` (`svc_webview_history`) | Managed via `db/tsdb/sql/*.sql` (with `-- NO_TRANSACTION` on policy/CAGG DDL) |

### 5.2 Core Entities

- **`platform_iam` (`core-iam`):**
  - `users`: `id`, `email` (citext unique), `password_hash` (argon2id, nullable while `INVITED`), `status` (`INVITED | ACTIVE | DISABLED`), `is_platform_admin`, `created_at`, `updated_at`.
  - `user_tokens`: `id`, `user_id`, `type` (`INVITE | PASSWORD_RESET`), `token_hash` (unique), `expires_at`, `used_at`, `created_at`.
  - `refresh_tokens`: `id`, `user_id`, `token_hash` (unique), `expires_at`, `revoked_at`, `replaced_by_id`, `client_info` (jsonb), `created_at`.
  - `sites`: `id`, `name`, `slug` (unique), `wireguard_conf_encrypted` (bytea, AES-256-GCM with `WG_CONF_ENCRYPTION_KEY`), `wireguard_meta` (jsonb: `publicKey`, `clientPublicKey`, `endpointHost`, `endpointPort`, `lanSubnets[]`, `confFingerprint`, `mtu`, `persistentKeepalive`), `vnc_enabled`, `webview_enabled`, `is_active`, `status` (`ONLINE | DEGRADED | OFFLINE | UNKNOWN`), `handshake_state`, `last_handshake_at`, `status_updated_at`, `created_at`, `updated_at`.
  - `site_memberships`: `id`, `user_id`, `site_id`, `role` (`SITE_OPERATOR | SITE_MEMBER`), unique(`user_id`, `site_id`).
- **`platform_runners` (`runner-orchestrator`):**
  - `runners`: `id`, `site_id` (unique), `container_name`, `container_id`, `image`, `conf_fingerprint`, `auth_token_encrypted`, `desired_state` (`RUNNING | STOPPED`), `observed_state`, `status`, `handshake_state`, `handshake_age_seconds`, `last_handshake_at`, `last_checked_at`, `last_error`, `last_reconciled_at`.
  - `runner_health_events`: transition-only audit of status changes (`id`, `site_id`, `from_status`, `to_status`, `occurred_at`, `detail`).
- **`platform_webvnc` (`webvnc-proxy`):**
  - `vnc_targets`: `id`, `site_id`, `name`, `host` (`INET`, validated ∈ `lanSubnets`), `port` (default 5900), `password_encrypted` (bytea, `VNC_PASSWORD_ENCRYPTION_KEY`, write-only), `is_active`.
  - `vnc_sessions`: `id`, `site_id`, `target_id`, `user_id`, `started_at`, `ended_at`, `bytes_in`, `bytes_out`, `close_reason`.
- **`platform_webview` (`webview-engine`):**
  - `devices`: `id`, `site_id`, `name`, `host` (`INET`, validated ∈ `lanSubnets`), `port`, `protocol` (`MODBUS_TCP | MODBUS_UDP`), `unit_id`, `max_concurrent_requests` (default 1), `request_timeout_ms` (default 2000), `is_active`.
  - `register_mappings`: `id`, `device_id`, `site_id`, `name`, `function_code` (`READ_COIL | READ_DISCRETE | READ_HOLDING | READ_INPUT`), `register_address`, `register_count`, `data_type` (`BOOL | UINT16 | INT16 | UINT32 | INT32 | FLOAT32`), `word_order` (`BIG | LITTLE`), `byte_order` (`BIG | LITTLE`), `scale`, `offset`, `unit`, `access_mode` (`READ | WRITE | READ_WRITE`), `write_function_code` (`WRITE_COIL | WRITE_REGISTER | WRITE_MULTIPLE_REGISTERS`), `interactive_poll_interval_ms` (default 1000), `archival_poll_interval_ms` (default 60000), `deadband` (default 0), `max_interval_seconds` (default 300), `is_active`.
  - `dashboards`: `id`, `site_id`, `name`, `layout` (jsonb), `is_default`.
  - `widgets`: `id`, `dashboard_id`, `register_mapping_id`, `name`, `widget_type` (`lamp | display | switch | button | number_input | chart`), `order`, `style` (jsonb), `extra` (jsonb).
  - `alarm_rules`, `alarm_events`, `alarm_pending_states`: latch/re-arm rules with `clear_deadband`, `hold_time_seconds`, `notify_email`, `notify_on_clear`, `recipient_emails`, persisted pending holds, `is_stale`, `acked_at`/`acked_by`, `notified_at`/`notify_failed`.
- **`platform_webview_history` (Dedicated TimescaleDB):**
  - `data_value_histories(time TIMESTAMPTZ, register_mapping_id UUID, site_id UUID, value DOUBLE PRECISION)` — 1-day chunks, compressed after 7 days (segmented by `site_id, register_mapping_id`), retained 30 days.
  - Continuous aggregates `data_value_1m` (`<= 7d`) and `data_value_1h` (`> 7d`) storing `avg`, `min`, `max`, `last(value, time)`, `count`, retained 1 year. Queried with `time_bucket_gapfill` + `locf()` for deadband continuity.

---

## 6. Services & Internal Contracts

### 6.1 `core-iam`
- Owns user/invite/password lifecycle, RS256 JWKS (`kid` rotation), site lifecycle, WireGuard conf validation/narrowing/encryption, and 15 s status projection (`LeaderElector`-guarded).
- **Internal `:8081` endpoints:**
  - `GET /internal/sites/{siteId}/authz?user_id=` → `{ role, isPlatformAdmin, isActive, status, vncEnabled, webviewEnabled, historyEnabled, lanSubnets }`
  - `GET /internal/sites/runtime-state` → lightweight slice of all sites' `{ siteId, isActive, status, vncEnabled, webviewEnabled, historyEnabled, lanSubnets }` (polled every 15 s by `webview-engine` and `webvnc-proxy`).
  - `GET /internal/runners/desired` → active/stopped desired runner specs (`siteId`, `desiredState`, `confFingerprint`).
  - `GET /internal/sites/{siteId}/runner-conf` → decrypted normalized WireGuard config for tmpfs recovery on reboot or reconcile.

### 6.2 `runner-orchestrator`
- Implements `RunnerBackend` (Docker SDK in v1; `client-go` on K8s).
- Creates `site-runner-<site_id>` on `wiredeck-runners` with `NET_ADMIN`, `:ro` tmpfs conf mount, per-runner `WIRERELAY_AUTH_TOKEN`, `DISABLE_IPV6=true`, memory/PID limits, no published ports.
- Binds internal HTTP API strictly on `wiredeck-control`: `PUT/DELETE/GET /runners/{siteId}`, `GET /runners`, `POST /runners/{siteId}/restart`.
- Before recreating on conf/image change, calls `POST /internal/sites/{siteId}/drain` on `webview-engine:8081` and `webvnc-proxy:8081` (max 10 s wait).
- 60 s reconciliation loop (`LeaderElector`-guarded): self-heals missing containers, image drift, and empty tmpfs after host reboot (via `core-iam:8081`).
- 30 s concurrent health loop over persistent gRPC channels (with `WIRERELAY_AUTH_TOKEN` metadata) → records `runners` + transition-only `runner_health_events`.

### 6.3 `webvnc-proxy`
- Public `:8080`: Target CRUD under `/api/v1/vnc/sites/{siteId}/admin/targets`; operator target list + `POST /api/v1/vnc/sites/{siteId}/ws-ticket` + `WS /api/v1/vnc/stream?ticket=`.
- **Server-side RFB 3.8 auth (D25):** When a target has `password_encrypted`, proxy completes RFB protocol version + SecurityType 2 DES challenge-response with the target, negotiates SecurityType 1 (None) with browser `noVNC`, then pumps raw bytes full-duplex over `WireRelay.TunnelStream`.
- Enforces `VNC_IDLE_TIMEOUT` (`15m`), `VNC_MAX_SESSION` (`8h`), runtime-state disconnect if `vncEnabled`/`isActive` flips false, and `:8081/internal/sites/{siteId}/drain` + `DELETE /internal/sites/{siteId}/data`.

### 6.4 `webview-engine`
- Single replica in v1 (`OwnershipProvider` seam checked before polling/alarm/ingest work).
- **Demand-Driven Scheduler (D17):** Effective interval = `min(archival, alarm, interactive)`; skips `access_mode = WRITE` and zero-demand mappings; prime-on-subscribe, 30 s interactive grace, 5 m idle device teardown.
- **Native Go MBAP Codec & Coalescer (D26):** 1 persistent `TunnelStream` per `Device`, per-device serial queue, contiguous coalescing (≤125 regs / ≤2000 bits), FC `0x01`/`0x02`/`0x03`/`0x04`/`0x05`/`0x06`/`0x0F`/`0x10`, exponential backoff (`1s → 60s`), `GOOD | STALE | BAD` quality.
- **History Ingester (`HistoryStore`):** Deadband + `max_interval_seconds`, buffered in memory (flush every 1,000 ms or 500 rows + guaranteed `SIGTERM` flush) using `pgx.CopyFrom` into dedicated TimescaleDB.
- **Alarm Engine (D10, D12):** `NORMAL -> PENDING -> ACTIVE -> NORMAL` with `clear_deadband`, persisted `alarm_pending_states`, site-OFFLINE freeze (`is_stale = true` + cancel pending), `MailGopher` notifications, live `ALARM_EVENT` WS broadcast.
- Internal `:8081`: `POST /internal/sites/{siteId}/drain`, `DELETE /internal/sites/{siteId}/data` (purges OLTP + TSDB site data).

### 6.5 Side Helpers (`WireRelay` v1.1+ & `MailGopher` v1.2+)
- **WireRelay R1–R11:** `TunnelClose` EOF frame, zero-leak stream cancellation, gRPC keepalive, `MaxConcurrentStreams`, `WIRERELAY_AUTH_TOKEN`, DDNS periodic re-resolve, `HandshakeState` + multi-peer worst-state health, `:ro`-safe `/run/wireguard` entrypoint copy, REST `/health` header fix, shutdown-aware `TunnelStream`, `ip_forward=0` + `FORWARD DROP`.
- **MailGopher M1–M3:** Panic-free graceful shutdown retries, burst-1 immediate first-send pacing (no 72 s idle delay for invites/alarms), startup validation of `limit_per_hour >= 1`.

---

## 7. Frontend — Three-Surface Modular Vue 3.5 SPA

Built with **Vue 3.5 + TypeScript + Vite 8 + Pinia + Tailwind CSS v4 + ECharts + `@novnc/novnc`**, using `openapi-typescript` generated types from Huma v2 OpenAPI specs.

| Surface | Layout & Chunk | Routes | Role Guard |
|---|---|---|---|
| **Service / Member Shell** | `OperatorLayout.vue` (eager) | `/sites/:siteId/{overview, webview, vnc, history, alarms}`, `/alarms`, `/profile` | Authenticated site member |
| **Site Operator Workspace** | `SiteAdminLayout.vue` (lazy chunk) | `/sites/:siteId/manage/{members, vnc-targets, webview}` | `SITE_OPERATOR` or `PLATFORM_ADMIN` |
| **Platform Admin Console** | `PlatformAdminLayout.vue` (lazy chunk) | `/admin/{sites, users, runners}` | `isPlatformAdmin` |

- **Telemetry WebSocket:** Single multiplexed connection per tab (`shallowRef` value/quality map), 15 s ping/pong heartbeat, `visibilitychange` auto-unsubscribe/resubscribe, 900-pt live ring buffer for `HistoryChart` when `history_enabled = false`.
- **Widgets (D27):** `WidgetLamp`, `WidgetDisplay`, `WidgetSwitch`, `WidgetButton`, `WidgetNumberInput`, `HistoryChart` — all rendering explicit `STALE`/`BAD` visual states when `quality !== 'GOOD'`.

---

## 8. Phased Roadmap

- **Phase 0:** `WireRelay` R1–R11 (`v1.1.0`), `MailGopher` M1–M3 (`v1.2.0`), `WireDeck2` repo init, v2.2 docs, split `db/oltp` (PG18) + `db/tsdb` (TimescaleDB) migrators, segmented Compose & dev simulators.
- **Phase 1:** Shared `pkg/`, `core-iam`, `runner-orchestrator`, Traefik/nginx, SPA auth + Three-Surface shells + WG conf upload/narrowing + email invites.
- **Phase 2:** `webvnc-proxy` (server-side RFB 3.8 auth, ticket WS, drain) + `VncView` + Site Admin VNC Targets.
- **Phase 3:** `webview-engine` (native Go MBAP codec, stream pool, coalescer, demand scheduler, WS + REST writes) + full 6-widget dashboard + Device/Register/Dashboard editors.
- **Phase 4:** TimescaleDB `HistoryStore` (deadband + `pgx.CopyFrom` + `locf()` rollups), Alarm Engine + MailGopher, `HistoryView` + `AlarmCenter`, Prometheus `/metrics`, `RUNBOOK.md`, CI/CD workflows.
