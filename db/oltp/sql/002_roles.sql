-- 002_roles.sql
-- One Postgres role per OLTP service, owning strictly its own schema.
-- Cross-domain reads are enforced through internal HTTP APIs (:8081).

SELECT 'CREATE ROLE svc_core_iam LOGIN'
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'svc_core_iam') \gexec
SELECT 'CREATE ROLE svc_runner_orchestrator LOGIN'
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'svc_runner_orchestrator') \gexec
SELECT 'CREATE ROLE svc_webvnc_proxy LOGIN'
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'svc_webvnc_proxy') \gexec
SELECT 'CREATE ROLE svc_webview_engine LOGIN'
 WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'svc_webview_engine') \gexec

ALTER ROLE svc_core_iam             WITH PASSWORD :'iam_password';
ALTER ROLE svc_runner_orchestrator  WITH PASSWORD :'orchestrator_password';
ALTER ROLE svc_webvnc_proxy         WITH PASSWORD :'webvnc_password';
ALTER ROLE svc_webview_engine       WITH PASSWORD :'webview_password';

ALTER SCHEMA platform_iam      OWNER TO svc_core_iam;
ALTER SCHEMA platform_runners  OWNER TO svc_runner_orchestrator;
ALTER SCHEMA platform_webvnc   OWNER TO svc_webvnc_proxy;
ALTER SCHEMA platform_webview  OWNER TO svc_webview_engine;

-- Prevent misconfigured connection strings from quietly creating tables in public.
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Lock each service role's default search_path to its own schema (+ public for extensions).
ALTER ROLE svc_core_iam             SET search_path = platform_iam, public;
ALTER ROLE svc_runner_orchestrator  SET search_path = platform_runners, public;
ALTER ROLE svc_webvnc_proxy         SET search_path = platform_webvnc, public;
ALTER ROLE svc_webview_engine       SET search_path = platform_webview, public;
