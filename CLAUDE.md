# CLAUDE.md — WireDeck2

Guidance for working in the `WireDeck2` repository. Always consult `docs/ARCHITECTURE.md` (v2.2) and `docs/k8s-host.md` before changing boundaries or contracts.

## Repository Layout

- `pkg/` — Shared Go packages imported by all 4 services (Huma v2 + Chi HTTP/OpenAPI setup, RS256 JWKS & internal Bearer auth, AES-256-GCM crypto, WireGuard conf parser/normalizer, WireRelay gRPC client/proto, structured `slog` + Prometheus telemetry, K8s `LeaderElector` seam).
- `services/core-iam/` — Go + Bun + Huma v2. Identity, admin email invites, self-service password change/reset, RS256 JWKS, site lifecycle, WG conf ingestion/encryption, status projection.
- `services/runner-orchestrator/` — Go + Bun. Sole holder of `/var/run/docker.sock` behind `RunnerBackend`; binds HTTP strictly on `wiredeck-control`.
- `services/webvnc-proxy/` — Go + Bun + Huma v2. Server-side RFB 3.8 VNC auth proxy + WS ↔ `WireRelay.TunnelStream` bridge + target CRUD.
- `services/webview-engine/` — Go + Bun + Huma v2 (single replica in v1, `OwnershipProvider` seam). Native MBAP Modbus codec, per-device stream pool & coalescer, demand-driven scheduler, TimescaleDB `HistoryStore`, latch/re-arm alarm engine.
- `frontend/` — Vue 3.5 + TypeScript + Vite 8 + Pinia + Tailwind CSS v4 + ECharts + `@novnc/novnc`. Three-surface modular SPA (`OperatorLayout`, lazy `SiteAdminLayout`, lazy `PlatformAdminLayout`).
- `db/oltp/` & `db/tsdb/` — One-shot bootstrap migrators for PostgreSQL 18 (OLTP) and TimescaleDB 2.32-pg18 (TSDB). Per-service OLTP tables live in embedded `goose` migrations inside each service.
- `deploy/` — Production & dev Docker Compose stacks, Traefik v3.7 configs, nginx SPA config, `.env.example`.

## Non-Negotiable Architectural Rules

1. **100% Schema-Validated HTTP + OpenAPI 3.1:** Every public route uses Huma v2 typed input/output structs (`additionalProperties: false`). Never bypass schema validation with raw `json.NewDecoder` on public endpoints.
2. **Strict 3-Role Separation (`Platform Admin`, `Site Operator`, `Site Member`):** Platform Admin routes live exclusively under `/api/v1/iam/admin/*` (`RequirePlatformAdmin` — sole owner of sites, `vnc_enabled`/`webview_enabled` flags, WireGuard configs, runners, global users). Site Operator routes live under `/api/v1/*/sites/{siteId}/operator/*` (`RequireSiteOperator` — site member invites as `SITE_MEMBER`, VNC edge points, WebView pages/elements/history/alarms). Fine-grained member policies live inside each service. Internal routes live on `:8081/internal/*` (`INTERNAL_API_TOKEN`, never routed by Traefik).
3. **Network & Key Boundaries:** Site runners live ONLY on `wiredeck-runners`. `WG_CONF_ENCRYPTION_KEY` is exclusive to `core-iam`; `VNC_PASSWORD_ENCRYPTION_KEY` is exclusive to `webvnc-proxy`.
4. **Database Boundaries:** Each OLTP service connects as its own Postgres role owning only its own schema. Cross-domain data goes via `:8081` internal HTTP APIs. Time-series history always goes through `HistoryStore` on the dedicated TimescaleDB instance.
5. **K8s Seams:** Never leak Docker container IDs, host paths, or localhost URLs across service boundaries. Address runners solely as `site-runner-<site_id>:9000`.
6. **UI Guidelines:**
   - Never use em-dash (`—`), en-dash (`–`), or other unicode line characters in UI copy (only ASCII `-` and `_` are allowed).
   - Platform Admin site service enable/disable controls must use a dynamic list-based view (not hardcoded checkboxes) so new services can be added cleanly.
