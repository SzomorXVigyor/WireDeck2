#!/bin/sh
# One-shot migrator for postgres-tsdb (TimescaleDB 2.32.1 on PostgreSQL 18.7).
# Creates the timescaledb extension, history role, hypertable, compression,
# continuous aggregates, and retention policies.
set -eu

: "${TSDB_DATABASE_URL:?TSDB_DATABASE_URL is required}"
: "${TSDB_HISTORY_PASSWORD:?TSDB_HISTORY_PASSWORD is required}"

SQL_DIR="${SQL_DIR:-/app/sql}"
PSQL="psql --no-psqlrc --set ON_ERROR_STOP=1 --quiet"

echo "[tsdb-migrate] waiting for postgres-tsdb..."
until psql "$TSDB_DATABASE_URL" -c 'SELECT 1' >/dev/null 2>&1; do
  sleep 1
done

$PSQL "$TSDB_DATABASE_URL" <<'SQL'
CREATE TABLE IF NOT EXISTS public.schema_migrations (
  filename    TEXT PRIMARY KEY,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

for file in "$SQL_DIR"/*.sql; do
  name=$(basename "$file")

  applied=$(psql "$TSDB_DATABASE_URL" --no-psqlrc -tAc \
    "SELECT 1 FROM public.schema_migrations WHERE filename = '$name'")

  if [ "$applied" = "1" ]; then
    echo "[tsdb-migrate] skip    $name"
    continue
  fi

  echo "[tsdb-migrate] apply   $name"
  if grep -q '^-- NO_TRANSACTION' "$file"; then
    $PSQL "$TSDB_DATABASE_URL" \
      -v history_password="$TSDB_HISTORY_PASSWORD" \
      -f "$file"
  else
    $PSQL "$TSDB_DATABASE_URL" --single-transaction \
      -v history_password="$TSDB_HISTORY_PASSWORD" \
      -f "$file"
  fi

  $PSQL "$TSDB_DATABASE_URL" -c \
    "INSERT INTO public.schema_migrations (filename) VALUES ('$name')"
done

echo "[tsdb-migrate] done"
