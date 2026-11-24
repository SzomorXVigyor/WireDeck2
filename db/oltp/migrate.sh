#!/bin/sh
# One-shot bootstrap migrator for postgres-oltp (PostgreSQL 18.7).
# Creates extensions, per-service schemas, and least-privilege service roles.
# Each Go service runs its own embedded goose table migrations on startup as its own role.
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"
: "${IAM_DB_PASSWORD:?IAM_DB_PASSWORD is required}"
: "${ORCHESTRATOR_DB_PASSWORD:?ORCHESTRATOR_DB_PASSWORD is required}"
: "${WEBVNC_DB_PASSWORD:?WEBVNC_DB_PASSWORD is required}"
: "${WEBVIEW_DB_PASSWORD:?WEBVIEW_DB_PASSWORD is required}"

SQL_DIR="${SQL_DIR:-/app/sql}"
PSQL="psql --no-psqlrc --set ON_ERROR_STOP=1 --quiet"

echo "[oltp-migrate] waiting for postgres-oltp..."
until psql "$DATABASE_URL" -c 'SELECT 1' >/dev/null 2>&1; do
  sleep 1
done

$PSQL "$DATABASE_URL" <<'SQL'
CREATE TABLE IF NOT EXISTS public.schema_migrations (
  filename    TEXT PRIMARY KEY,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

for file in "$SQL_DIR"/*.sql; do
  name=$(basename "$file")

  applied=$(psql "$DATABASE_URL" --no-psqlrc -tAc \
    "SELECT 1 FROM public.schema_migrations WHERE filename = '$name'")

  if [ "$applied" = "1" ]; then
    echo "[oltp-migrate] skip    $name"
    continue
  fi

  echo "[oltp-migrate] apply   $name"
  if grep -q '^-- NO_TRANSACTION' "$file"; then
    $PSQL "$DATABASE_URL" \
      -v iam_password="$IAM_DB_PASSWORD" \
      -v orchestrator_password="$ORCHESTRATOR_DB_PASSWORD" \
      -v webvnc_password="$WEBVNC_DB_PASSWORD" \
      -v webview_password="$WEBVIEW_DB_PASSWORD" \
      -f "$file"
  else
    $PSQL "$DATABASE_URL" --single-transaction \
      -v iam_password="$IAM_DB_PASSWORD" \
      -v orchestrator_password="$ORCHESTRATOR_DB_PASSWORD" \
      -v webvnc_password="$WEBVNC_DB_PASSWORD" \
      -v webview_password="$WEBVIEW_DB_PASSWORD" \
      -f "$file"
  fi

  $PSQL "$DATABASE_URL" -c \
    "INSERT INTO public.schema_migrations (filename) VALUES ('$name')"
done

echo "[oltp-migrate] done"
