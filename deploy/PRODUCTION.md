# WireDeck Deployment Guide (Prod, Staging, Local)

Production and Staging pull prebuilt images from GitHub Container Registry (`ghcr.io/szomorxvigyor/wiredeck2/*`) — no source code, Go toolchain, or Node.js is needed on those hosts. Local builds all 7 WireDeck images directly from this repository.

| Environment | Compose File | Env Template | Images | TLS / Certs | Mail |
|---|---|---|---|---|---|
| **Production** | `deploy/docker-compose.prod.yml` | `deploy/.env.prod.example` | GHCR (`ghcr.io`) | Built-in Traefik Let's Encrypt (`80`/`443`) | Real SMTP via MailGopher |
| **Staging** | `deploy/docker-compose.staging.yml` | `deploy/.env.staging.example` | GHCR (`ghcr.io`) | **External proxy** terminates TLS on `443` and forwards HTTP + WS to `STAGING_HTTP_BIND` (`127.0.0.1:8080`) | Real SMTP via MailGopher |
| **Local** | `deploy/docker-compose.local.yml` | `deploy/.env.local.example` | **Local source builds** | **None** — plain HTTP on `http://localhost:8080` | Built-in **Mailpit** (`http://localhost:18025`) |

---

## 1. Files to Copy to the Target Host

You only need **3 files** from `deploy/` per environment:

### Production (`/opt/wiredeck/`)
```text
/opt/wiredeck/
├── docker-compose.prod.yml   <- deploy/docker-compose.prod.yml
├── .env                      <- deploy/.env.prod.example (filled with secrets)
└── traefik/
    └── dynamic.yml           <- deploy/traefik/dynamic.yml
```

### Staging (`/opt/wiredeck-staging/`)
```text
/opt/wiredeck-staging/
├── docker-compose.staging.yml  <- deploy/docker-compose.staging.yml
├── .env                        <- deploy/.env.staging.example (filled with secrets)
└── traefik/
    └── dynamic.yml             <- deploy/traefik/dynamic.yml
```

### Local (Run directly from this repo)
No copying needed — copy `deploy/.env.local.example` to `deploy/.env.local` and run from the repo root.

---

## 2. Host Setup (Production & Staging)

> Local testing (`docker-compose.local.yml`) creates its own Docker networks and uses `/tmp/wiredeck-local-runners` automatically — just `mkdir -p /tmp/wiredeck-local-runners` and skip to Step 3.

### A. Create the Two Isolated Docker Networks (Prod & Staging)
Leaves `172.21.0.0/16` and `172.22.0.0/16` free for remote site LANs:
```bash
docker network create --driver bridge --internal --subnet=172.30.248.0/24 --ipv6=false wiredeck-control
docker network create --driver bridge --subnet=172.30.252.0/22 --ipv6=false wiredeck-runners
```

### B. Mount `tmpfs` for WireGuard Runner Configs (Prod & Staging)
Keeps site WireGuard private keys in RAM only (`runner-orchestrator` repopulates automatically from `core-iam` after reboot):
```bash
sudo mkdir -p /var/lib/wiredeck2/runners
sudo mount -t tmpfs -o size=16m,mode=0700 tmpfs /var/lib/wiredeck2/runners
```
Persist in `/etc/fstab`:
```text
tmpfs /var/lib/wiredeck2/runners tmpfs size=16m,mode=0700 0 0
```

---

## 3. Generate Keys & Configure `.env`

Generate the RSA signing key and four distinct 32-byte secrets:
```bash
# Paste into JWT_PRIVATE_KEY_B64
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 | base64 -w0; echo

# Paste into the matching keys
for k in WG_CONF_ENCRYPTION_KEY VNC_PASSWORD_ENCRYPTION_KEY RUNNER_TOKEN_ENCRYPTION_KEY INTERNAL_API_TOKEN; do
  echo "${k}=$(openssl rand -base64 32)"
done
```
Set `chmod 600 .env`, pin `WIREDECK_VERSION` to a GitHub Release tag (e.g. `v26.11-241200`), and fill in `ROOT_DOMAIN`, DB passwords, and `SMTP_ACCOUNTS` (prod/staging).

If GHCR packages are private, log in once on the host:
```bash
echo "<GITHUB_PAT_WITH_READ_PACKAGES>" | docker login ghcr.io -u <GITHUB_USERNAME> --password-stdin
```

---

## 4. Upstream Reverse Proxy Config (Staging Only)

Point your outer TLS-terminating proxy (`https://${ROOT_DOMAIN}` on `:443`) to `http://127.0.0.1:8080` (or your `STAGING_HTTP_BIND`). **WebSocket upgrade headers are required** for live WebView telemetry (`/api/v1/webview/stream`) and Browser VNC (`/api/v1/vnc/stream`).

### Nginx example
```nginx
server {
    listen 443 ssl http2;
    server_name staging.wiredeck.example.com;

    # ssl_certificate / ssl_certificate_key managed by your outer proxy

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}
```

### Caddy example
```caddy
staging.wiredeck.example.com {
    reverse_proxy 127.0.0.1:8080
}
```
*(Caddy forwards `Host`, `X-Forwarded-*`, and WebSocket upgrades automatically.)*

---

## 5. Start & Upgrade Commands

**Production:**
```bash
cd /opt/wiredeck
docker compose -f docker-compose.prod.yml --env-file .env pull
docker compose -f docker-compose.prod.yml --env-file .env up -d
```

**Staging:**
```bash
cd /opt/wiredeck-staging
docker compose -f docker-compose.staging.yml --env-file .env pull
docker compose -f docker-compose.staging.yml --env-file .env up -d
```
Staging includes **Watchtower** (`nickfedor/watchtower:1.24.1`, opt-in scoped to `wiredeck-staging`). With `WIREDECK_VERSION=latest`, Watchtower polls GHCR every `WATCHTOWER_POLL_INTERVAL` seconds (default `300s`) and automatically pulls, restarts, and cleans up old images for `frontend`, `core-iam`, `runner-orchestrator`, `webview-engine`, and `webvnc-proxy` whenever GitHub Actions publishes new images — without touching Postgres, TimescaleDB, Traefik, or site runners. Set `DOCKER_CONFIG_DIR` in `.env` to the host `.docker` folder where you ran `docker login ghcr.io` if your GHCR packages are private.

**Local (builds from local source):**
```bash
mkdir -p /tmp/wiredeck-local-runners && chmod 700 /tmp/wiredeck-local-runners
docker compose -f deploy/docker-compose.local.yml --env-file deploy/.env.local up -d --build
# App: http://localhost:8080  |  Mailpit: http://localhost:18025
```

---

## 6. Database Backups (Prod & Staging)

```bash
# Replace container name prefix with wiredeck-staging-* for staging
docker exec wiredeck-postgres-oltp pg_dump -U postgres -Fc wiredeck_oltp > wiredeck_oltp_$(date +%F).dump
docker exec wiredeck-postgres-tsdb pg_dump -U postgres -Fc wiredeck_history > wiredeck_history_$(date +%F).dump
```
