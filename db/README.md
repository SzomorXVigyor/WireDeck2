# Database Architecture (OLTP + Dedicated TSDB)

WireDeck2 uses **two physically separated PostgreSQL instances** on the internal `wiredeck-control` network:

1. **OLTP (`postgres-oltp`, `postgres:18-alpine`)** — relational platform state, segmented into 4 logical schemas with **one Postgres role per service**, owning only its own schema.
2. **TSDB (`postgres-tsdb`, `timescale/timescaledb:2.32.0-pg17`)** — dedicated time-series database for remote sensor and register history (`platform_webview_history` hypertable, compression, `1m` & `1h` continuous aggregates, retention policies).

## Why Split OLTP and TSDB?

- **Blast-radius isolation:** High-volume sensor writes, continuous aggregate refreshes, and wide range scans never compete for WAL, I/O, or locks with IAM authentication or control-plane writes.
- **Independent upgrades & backups:** OLTP runs the latest vanilla `postgres:18-alpine` with trivial `pg_dump` backups; TSDB uses `timescale/timescaledb:2.32.0-pg17` with physical backups (`pgBackRest` / volume snapshots).
- **K8s portability:** `webview-engine` writes/reads time-series strictly through a Go `HistoryStore` interface.

## Who Migrates What

| Database | Schema | Bootstrap Migrator | Table / Schema Migrations |
|---|---|---|---|
| `postgres-oltp` | `platform_iam` | `db/oltp` (extensions, schema, `svc_core_iam`) | `services/core-iam` (embedded `goose`) |
| `postgres-oltp` | `platform_runners` | `db/oltp` (schema, `svc_runner_orchestrator`) | `services/runner-orchestrator` (embedded `goose`) |
| `postgres-oltp` | `platform_webvnc` | `db/oltp` (schema, `svc_webvnc_proxy`) | `services/webvnc-proxy` (embedded `goose`) |
| `postgres-oltp` | `platform_webview` | `db/oltp` (schema, `svc_webview_engine`) | `services/webview-engine` (embedded `goose`) |
| `postgres-tsdb` | `platform_webview_history` | `db/tsdb` (TimescaleDB ext, role, hypertable, CAGGs, policies) | `db/tsdb/sql/*.sql` |

## Adding a Raw Bootstrap Migration

Add `00N_name.sql` under `db/oltp/sql/` or `db/tsdb/sql/`; never edit an applied file.
If a file calls TimescaleDB policy/CAGG DDL (`add_compression_policy`, `add_retention_policy`, `CREATE MATERIALIZED VIEW ... WITH (timescaledb.continuous)`), place `-- NO_TRANSACTION` on line 1.
