-- 001_schema_and_role.sql
-- TimescaleDB extension, dedicated history schema, and least-privilege role.

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE SCHEMA IF NOT EXISTS platform_webview_history;

SELECT 'CREATE ROLE svc_webview_history LOGIN'
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'svc_webview_history') \gexec

ALTER ROLE svc_webview_history WITH PASSWORD :'history_password';
ALTER SCHEMA platform_webview_history OWNER TO svc_webview_history;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO svc_webview_history;
ALTER ROLE svc_webview_history SET search_path = platform_webview_history, public;
