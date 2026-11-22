-- +goose Up
CREATE TABLE IF NOT EXISTS users (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email             CITEXT NOT NULL UNIQUE,
  password_hash     TEXT,
  status            TEXT NOT NULL DEFAULT 'INVITED' CHECK (status IN ('INVITED', 'ACTIVE', 'DISABLED')),
  is_platform_admin BOOLEAN NOT NULL DEFAULT FALSE,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_tokens (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type        TEXT NOT NULL CHECK (type IN ('INVITE', 'PASSWORD_RESET')),
  token_hash  TEXT NOT NULL UNIQUE,
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_tokens_user_idx ON user_tokens (user_id, type);

CREATE TABLE IF NOT EXISTS refresh_tokens (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash     TEXT NOT NULL UNIQUE,
  expires_at     TIMESTAMPTZ NOT NULL,
  revoked_at     TIMESTAMPTZ,
  replaced_by_id UUID,
  client_info    JSONB,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS refresh_tokens_user_idx ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_expires_idx ON refresh_tokens (expires_at);

CREATE TABLE IF NOT EXISTS sites (
  id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                     TEXT NOT NULL,
  slug                     TEXT NOT NULL UNIQUE,
  wireguard_conf_encrypted BYTEA NOT NULL,
  wireguard_meta           JSONB NOT NULL,
  vnc_enabled              BOOLEAN NOT NULL DEFAULT FALSE,
  webview_enabled          BOOLEAN NOT NULL DEFAULT FALSE,
  is_active                BOOLEAN NOT NULL DEFAULT FALSE,
  status                   TEXT NOT NULL DEFAULT 'UNKNOWN' CHECK (status IN ('ONLINE', 'DEGRADED', 'OFFLINE', 'UNKNOWN')),
  handshake_state          TEXT NOT NULL DEFAULT 'UNSPECIFIED',
  last_handshake_at        TIMESTAMPTZ,
  status_updated_at        TIMESTAMPTZ,
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sites_active_idx ON sites (is_active);

CREATE TABLE IF NOT EXISTS site_memberships (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  site_id    UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  role       TEXT NOT NULL CHECK (role IN ('SITE_OPERATOR', 'SITE_MEMBER')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, site_id)
);
CREATE INDEX IF NOT EXISTS site_memberships_site_idx ON site_memberships (site_id);

-- +goose Down
DROP TABLE IF EXISTS site_memberships;
DROP TABLE IF EXISTS sites;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS user_tokens;
DROP TABLE IF EXISTS users;
