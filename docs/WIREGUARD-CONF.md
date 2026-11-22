# WireGuard Client Configuration Guide

WireDeck2 acts as a **WireGuard client** (D1, D2). Each remote site runs its own WireGuard server (e.g., MikroTik, OPNsense, pfSense, UniFi, OpenWrt, or Linux `wg-quick`) with a reachable UDP endpoint (static IP or DDNS + port forward).

When onboarding or rotating a site, **only a Platform Admin** uploads a standard WireGuard client `.conf` file. `core-iam` validates, normalises, and encrypts it with AES-256-GCM before storing it; the raw config and private key are **never** returned by any API or written to persistent runner disks, and non-secret WireGuard metadata is exposed exclusively to Platform Admins.

---

## Validation & Normalisation Rules

| Rule | Behaviour | Why |
|---|---|---|
| **Single `[Peer]` section** | Required (rejects 0 or >1 peers) | Multi-peer configs break unambiguous site tunnel health reporting. |
| **`PrivateKey` & `PublicKey`** | Must be valid 32-byte base64 WireGuard keys | Fails immediately at upload rather than silently at runner boot. |
| **`Endpoint = host:port`** | Required; hostname resolved at validation time | Catches typos/NXDOMAIN on upload. DDNS hostnames are automatically re-resolved every 60 s by `WireRelay`. |
| **`AllowedIPs` catch-all (`0.0.0.0/0`, `::/0`)** | **Narrow-don't-reject flow** | Most router exports default to `0.0.0.0/0`, which installs a default route inside the runner container and knocks it off `wiredeck-runners`. When detected, the API returns a structured `422` prompt and the UI asks for the site's actual LAN subnets (e.g., `192.168.10.0/24`), rewrites `AllowedIPs`, and saves them as `lanSubnets`. |
| **Overlap with `RESERVED_CIDRS`** | Rejected | Prevents `Address` or `AllowedIPs` from overlapping Docker bridges (`172.30.248.0/24`, `172.30.252.0/22`) or K8s Pod/Service CIDRs. |
| **Overlap with `Endpoint` IP** | Rejected | Prevents WireGuard from routing its own encrypted UDP packets into the tunnel. |
| **`Address` CIDR** | Accepts `/32`, `/128`, or private interface subnets (e.g., `/24`) if non-overlapping | Real-world router exports (MikroTik, OPNsense, `wg-easy`) commonly emit `/24`. |
| **`PersistentKeepalive`** | **Auto-injected as `25` if absent** | WireGuard only handshakes when traffic flows. Without keepalive, idle sites go stale and falsely report `OFFLINE`. |
| **`DNS = ...`** | Stripped automatically | `resolvconf` is unavailable inside minimal runner containers. |
| **`MTU`** | Validated in range `1280–1420` if present | Prevents fragmentation issues on PPPoE / LTE links. |

---

## Example Valid Client Config

```ini
[Interface]
PrivateKey = <base64-client-private-key>
Address = 10.200.1.2/32
MTU = 1380

[Peer]
PublicKey = <base64-site-server-public-key>
PresharedKey = <optional-base64-psk>
Endpoint = site-alpha.ddns.example.com:51820
AllowedIPs = 192.168.10.0/24, 10.200.1.0/24
PersistentKeepalive = 25
```

> **Note on `lanSubnets`:** Every Modbus `Device.host` and VNC `VncTarget.host` added to a site is validated to fall strictly inside that site's `AllowedIPs` (`lanSubnets`). Keep `AllowedIPs` tight to the actual automation/HMI subnets you want WireDeck2 to reach.
