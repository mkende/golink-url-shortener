# Configuration Reference

This document is derived from `config.template.toml`. See that file for a
copy-pasteable template with inline comments.

Pass the config file path to the server with the `-config` flag:

```bash
golink -config /etc/golink/golink.conf
```

All keys are optional unless marked **required**.

---

## Server

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `listen_addr` | string | `"0.0.0.0:8080"` | TCP address the HTTP server binds to (host:port). |
| `canonical_address` | string | `""` | Public base URL including scheme, e.g. `"https://go.example.com"` or `"http://go"`. Required when OIDC is enabled. When set, non-redirect requests on a different scheme or host are redirected here with 301. |
| `trusted_proxy` | list of strings | `[]` | CIDR ranges of trusted reverse proxies. When a request arrives from one of these IPs, `X-Forwarded-Proto` is trusted for scheme detection, and Tailscale/proxy-auth headers are accepted. Required when `proxy_auth.enabled = true`. |
| `title` | string | `"GoLink"` | Human-readable product name shown in the browser title and navigation bar. |
| `favicon_path` | string | `""` | Filesystem path to a custom favicon file (ICO, PNG, or SVG). Empty string uses the built-in default. |

---

## Authentication

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `require_auth_for_redirects` | bool | `false` | When `true`, unauthenticated users cannot follow any redirect and are sent to the login page instead. |

### `[tailscale]` — Tailscale header-based auth

Enable this when golink-url-shortener sits behind a Tailscale node that injects `Tailscale-User-*` headers. No additional credentials are needed.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `tailscale.enabled` | bool | `false` | Enable Tailscale header-based authentication. |

### `[oidc]` — OpenID Connect auth

The OAuth2 callback URL is always `<canonical_address>/auth/callback` and is
derived automatically — you do not set it in the config file. Register this URL
with your OIDC provider.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `oidc.enabled` | bool | `false` | Enable OIDC authentication. |
| `oidc.issuer` | string | `""` | OIDC provider issuer URL, e.g. `"https://accounts.google.com"`. Must match the `iss` claim in tokens. |
| `oidc.client_id` | string | `""` | OAuth2 client identifier issued by the provider. |
| `oidc.client_secret` | string | `""` | OAuth2 client secret. Keep this confidential. |
| `oidc.scopes` | []string | `["openid","email","profile"]` | OAuth2 scopes to request from the provider. Add `"groups"` to receive group membership claims (needed for `admin_group` and group-based sharing). |
| `oidc.groups_claim` | string | `"groups"` | Name of the JWT/userinfo claim that contains the user's group memberships. Used for group-based sharing and `admin_group`. |
| `oidc.jwt_secret` | string | `""` | **Required when `oidc.enabled = true`.** HMAC secret used to sign session JWT cookies. Use a long random string (at least 32 bytes). Generate one with `openssl rand -base64 32`. |

#### Authelia

Register a client in your Authelia configuration (`authelia/configuration.yml`):

```yaml
identity_providers:
  oidc:
    clients:
      - client_id: "golink"
        client_secret: "<bcrypt-hashed-secret>"   # authelia crypto hash generate --scheme bcrypt
        authorization_policy: one_factor           # or two_factor
        redirect_uris:
          - "https://go.example.com/auth/callback"
        scopes: ["openid", "email", "profile", "groups"]
        token_endpoint_auth_method: client_secret_basic
        userinfo_signed_response_alg: none
```

Then in `golink.conf`:

```toml
[oidc]
enabled      = true
issuer       = "https://authelia.example.com"
client_id    = "golink"
client_secret = "plaintext-secret"
scopes       = ["openid", "email", "profile", "groups"]
jwt_secret   = "replace-with-a-32-byte-random-string"
```

---

## Database

### `[db]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `db.driver` | string | `"sqlite"` | Database backend. Valid values: `"sqlite"`, `"postgres"`. |
| `db.dsn` | string | `"golink.db"` | Data source name / connection string. For SQLite: a file path (relative or absolute); the file is created if it does not exist. For PostgreSQL: a standard libpq connection string, e.g. `"host=localhost port=5432 dbname=golink user=golink password=secret sslmode=require"`. |

> **SQLite WAL mode** — The server automatically enables WAL (Write-Ahead Log) journal mode on every SQLite database at startup. This improves write throughput and crash durability. The WAL files (`golink.db-wal`, `golink.db-shm`) are normal SQLite artefacts; do not delete them while the server is running. No configuration is required.

---

## Links

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `quick_link_length` | int | `6` | Number of characters in a randomly-generated quick-link name. Must be `>= 4`. |
| `default_domain` | string | `""` | Domain appended to bare email addresses (without `@`) when resolving link share targets. For example, with `default_domain = "example.com"` sharing with `"alice"` is treated as `"alice@example.com"`. Disabled when empty. |
| `required_domain` | string | `""` | If set, link sharing is restricted to addresses in this domain only. Attempts to share outside this domain are rejected. Disabled when empty. |

---

## Performance

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `cache_size` | int | `1000` | Maximum number of links kept in the in-process LRU redirect cache. Hot links are served from memory without a database round-trip. Increase for workloads with many distinct popular links. |

---

## Admin

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `admin_emails` | []string | `[]` | Email addresses of users who have full admin privileges. Admins can manage API keys and run import/export operations. Example: `["alice@example.com", "bob@example.com"]`. |
| `admin_groups` | []string | `[]` | OIDC/proxy-auth group names whose members are treated as admins. Requires OIDC (or proxy_auth with groups) to be enabled and the groups_claim to be correctly configured. Example: `["sre", "platform-team"]`. |

---

## Minimal example

```toml
canonical_address = "https://go.example.com"

[oidc]
enabled      = true
issuer       = "https://auth.example.com"
client_id    = "golink"
client_secret = "secret"
jwt_secret   = "replace-with-a-32-byte-random-string"

admin_emails = ["alice@example.com"]
```
