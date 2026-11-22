-- 001_schemas.sql
-- OLTP extensions and logical schema segmentation (PostgreSQL 18).
-- See docs/ARCHITECTURE.md section 5.1.

CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS platform_iam;
CREATE SCHEMA IF NOT EXISTS platform_runners;
CREATE SCHEMA IF NOT EXISTS platform_webvnc;
CREATE SCHEMA IF NOT EXISTS platform_webview;
